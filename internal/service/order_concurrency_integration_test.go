package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
)

// Each goroutine writes its own result slot and starts behind the same barrier.
// Never use the mutable fake repository or sequential fixture ID generator here.
func concurrentOrderCommands(n int,fn func(int)error)[]error{
	start:=make(chan struct{});var wg sync.WaitGroup;result:=make([]error,n)
	for i:=0;i<n;i++{wg.Add(1);go func(i int){defer wg.Done();<-start;result[i]=fn(i)}(i)}
	close(start);wg.Wait();return result
}

type workflowFailureWork struct{repository.UnitOfWork;stage string;acquired,release chan struct{}}
type workflowFailureTx struct{repository.Tx;work *workflowFailureWork}
func(u workflowFailureWork)WithinTransaction(ctx context.Context,fn func(repository.Tx)error)error{
	return u.UnitOfWork.WithinTransaction(ctx,func(tx repository.Tx)error{return fn(workflowFailureTx{tx,&u})})
}
func(tx workflowFailureTx)LockOrderCommandKey(ctx context.Context,actor,key string)error{
	if err:=tx.Tx.LockOrderCommandKey(ctx,actor,key);err!=nil{return err}
	if tx.work.acquired!=nil{close(tx.work.acquired);select{case <-tx.work.release:case <-ctx.Done():return ctx.Err()}}
	return nil
}
func(tx workflowFailureTx)AppendAudit(ctx context.Context,e domain.AuditEvent)error{
	if err:=tx.Tx.AppendAudit(ctx,e);err!=nil{return err};if tx.work.stage=="audit"{return failure};return nil
}
func(tx workflowFailureTx)AppendOrderStatusEvent(ctx context.Context,e domain.OrderStatusEvent)error{
	if err:=tx.Tx.AppendOrderStatusEvent(ctx,e);err!=nil{return err};if tx.work.stage=="history"{return failure};return nil
}
func(tx workflowFailureTx)SaveOrderCommand(ctx context.Context,r repository.OrderCommandReceipt)error{
	if err:=tx.Tx.SaveOrderCommand(ctx,r);err!=nil{return err};if tx.work.stage=="receipt"{return failure};return nil
}

func TestPostgresOrderConcurrency(t *testing.T){
	db:=testutil.Postgres(t)
	ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
	if err:=migrate.Up(ctx,db);err!=nil{t.Fatal(err)}
	base:=postgres.New(db)
	hash,err:=bcrypt.GenerateFromPassword([]byte("synthetic-workflow-unused"),bcrypt.MinCost);if err!=nil{t.Fatal(err)}
	a:=domain.Staff{ID:"workflow-owner",Username:"workflow.test",Name:"Staf Contoh",Role:domain.Owner,Active:true,CreatedAt:fixed,UpdatedAt:fixed}
	c,err:=domain.NewCustomer("workflow-customer","Pelanggan Contoh","","",fixed);if err!=nil{t.Fatal(err)}
	s:=domain.Service{ID:"workflow-service",Name:"Cuci Contoh",Unit:domain.PerPiece,UnitPrice:10000,Turnaround:24*time.Hour,Active:true,CreatedAt:fixed,UpdatedAt:fixed}
	if err=base.WithinTransaction(ctx,func(tx repository.Tx)error{
		if err:=tx.AddStaff(ctx,a,string(hash));err!=nil{return err};if err:=tx.AddCustomer(ctx,c);err!=nil{return err};return tx.AddService(ctx,s)
	});err!=nil{t.Fatal(err)}
	rt:=Runtime{Clock:ClockFunc(func()time.Time{return fixed.Add(time.Hour)}),IDs:platform.RandomIDs{},Tokens:platform.RandomTokens{}}
	orders:=NewOrders(base,rt);payments:=NewPayments(base,rt)
	input:=func(key string)OrderInput{return OrderInput{IdempotencyKey:key,CustomerID:c.ID,Items:[]domain.ItemRequest{{ServiceID:s.ID,Quantity:domain.Quantity{Pieces:1}}}}}
	create:=func(key string)CreatedOrder{t.Helper();o,err:=orders.Create(ctx,a.ID,input(key));if err!=nil{t.Fatal(err)};return o}
	read:=func(id string)repository.OrderRecord{t.Helper();r,err:=orders.Get(ctx,a.ID,id);if err!=nil{t.Fatal(err)};return r}
	count:=func(query string,args ...any)int{t.Helper();var n int;if err:=db.QueryRowContext(ctx,query,args...).Scan(&n);err!=nil{t.Fatal(err)};return n}
	assertCounts:=func(id string,pay,ledger,event,receipt int){t.Helper()
		for _,tc:=range []struct{table string;want int}{{"payments",pay},{"ledger_entries",ledger},{"order_status_events",event},{"order_command_receipts",receipt}}{
			if got:=count("SELECT count(*) FROM public."+tc.table+" WHERE order_id=$1",id);got!=tc.want{t.Fatalf("%s: got %d want %d",tc.table,got,tc.want)}
		}
	}
	t.Run("duplicate_create_and_payload_conflict",func(t *testing.T){
		results:=make([]CreatedOrder,8)
		errs:=concurrentOrderCommands(len(results),func(i int)error{var err error;results[i],err=orders.Create(ctx,a.ID,input("duplicate-create"));return err})
		first:=0
		for i,err:=range errs{if err!=nil{t.Fatal(err)};if results[i].Order.ID()!=results[0].Order.ID(){t.Fatal("duplicate order")};if !results[i].Replayed{first++;if results[i].TrackingToken==""{t.Fatal("missing first capability")}}else if results[i].TrackingToken!=""{t.Fatal("capability replayed")}}
		if first!=1{t.Fatal("create applied more than once")}
		id:=results[0].Order.ID();assertCounts(id,0,0,1,1)
		if count(`SELECT count(*) FROM audit_events WHERE entity_id=$1 AND action='order_created'`,id)!=1{t.Fatal("duplicate create audit")}
		bad:=input("duplicate-create");bad.Notes="Payload berbeda"
		_,err:=orders.Create(ctx,a.ID,bad);requireDomain(t,err,domain.Conflict,"idempotency_conflict")
		_,err=orders.ChangeStatus(ctx,a.ID,id,domain.Processing,1,"duplicate-create");requireDomain(t,err,domain.Conflict,"idempotency_conflict")
	})
	t.Run("duplicate_status_and_replay_after_later_update",func(t *testing.T){
		o:=create("status-create");results:=make([]repository.OrderRecord,8)
		errs:=concurrentOrderCommands(8,func(i int)error{var err error;results[i],err=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"duplicate-status");return err})
		first:=0;for i,err:=range errs{if err!=nil{t.Fatal(err)};if !results[i].Replayed{first++};if results[i].Version!=2 || !results[i].Order.ProcessingAt().Equal(rt.Clock.Now()){t.Fatal("unstable milestone/version")}}
		if first!=1{t.Fatal("status applied more than once")}
		_,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Ready,2,"ready-status");if err!=nil{t.Fatal(err)}
		replay,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"duplicate-status")
		if err!=nil || !replay.Replayed || replay.Version!=3 || replay.AppliedVersion!=2 || replay.Order.Status()!=domain.Ready{t.Fatal("replay regressed state",err)}
		assertCounts(o.Order.ID(),0,0,3,3)
		if count(`SELECT count(*) FROM audit_events WHERE entity_id=$1 AND action='order_status_changed'`,o.Order.ID())!=2{t.Fatal("duplicate status audit")}
	})
	t.Run("lost_update_and_stale_version",func(t *testing.T){
		o:=create("lost-create")
		errs:=concurrentOrderCommands(2,func(i int)error{if i==0{_,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"lost-process");return err};_,err:=orders.Cancel(ctx,a.ID,o.Order.ID(),1,"Permintaan pelanggan","lost-cancel");return err})
		won:=0;for _,err:=range errs{if err==nil{won++}else{requireDomain(t,err,domain.Conflict,"stale_order")}}
		if won!=1{t.Fatal("lost update accepted")}
		r:=read(o.Order.ID());if r.Version!=2 || (r.Order.Status()!=domain.Processing && r.Order.Status()!=domain.Cancelled){t.Fatal("inconsistent winner")}
		_,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Ready,1,"stale-again");requireDomain(t,err,domain.Conflict,"stale_order")
		assertCounts(o.Order.ID(),0,0,2,2)
	})
	t.Run("invalid_concurrent_transitions_have_no_effect",func(t *testing.T){
		o:=create("invalid-create")
		errs:=concurrentOrderCommands(6,func(i int)error{_,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Ready,1,fmt.Sprintf("invalid-%d",i));return err})
		for _,err:=range errs{requireDomain(t,err,domain.Conflict,"invalid_transition")}
		if read(o.Order.ID()).Version!=1{t.Fatal("invalid transition persisted")};assertCounts(o.Order.ID(),0,0,1,1)
	})
	t.Run("duplicate_payment_has_one_ledger_and_no_laundry_transition",func(t *testing.T){
		o:=create("payment-create");in:=PaymentInput{OrderID:o.Order.ID(),IdempotencyKey:"duplicate-payment",Amount:10000,Method:domain.Cash}
		results:=make([]repository.PaymentResult,8)
		errs:=concurrentOrderCommands(8,func(i int)error{var err error;results[i],err=payments.Record(ctx,a.ID,in);return err})
		first:=0;for i,err:=range errs{if err!=nil{t.Fatal(err)};if !results[i].Replayed{first++};if results[i].Payment.ID!=results[0].Payment.ID{t.Fatal("duplicate payment identity")}}
		r:=read(o.Order.ID());if first!=1 || r.Version!=2 || r.Order.Status()!=domain.Received || r.Order.PaymentStatus()!=domain.Paid{t.Fatal("payment altered workflow or duplicate posting")}
		in.Method=domain.QRIS;_,err:=payments.Record(ctx,a.ID,in);requireDomain(t,err,domain.Conflict,"idempotency_conflict")
		assertCounts(o.Order.ID(),1,1,1,1)
		if count(`SELECT count(*) FROM audit_events WHERE action='payment_recorded' AND entity_id=$1`,results[0].Payment.ID)!=1{t.Fatal("duplicate payment audit")}
	})
	t.Run("payment_versus_cancel",func(t *testing.T){
		for trial:=0;trial<5;trial++{
			o:=create(fmt.Sprintf("cancel-race-create-%d",trial))
			errs:=concurrentOrderCommands(2,func(i int)error{if i==0{_,err:=payments.Record(ctx,a.ID,PaymentInput{OrderID:o.Order.ID(),IdempotencyKey:"cancel-race-payment",Amount:10000,Method:domain.Cash});return err};_,err:=orders.Cancel(ctx,a.ID,o.Order.ID(),1,"Permintaan pelanggan",fmt.Sprintf("cancel-race-%d",trial));return err})
			if (errs[0]==nil)==(errs[1]==nil){t.Fatalf("expected one winner: %v",errs)}
			r:=read(o.Order.ID())
			if errs[0]==nil{requireDomain(t,errs[1],domain.Conflict,"stale_order");if r.Order.Status()!=domain.Received || r.Order.Paid()!=10000{t.Fatal("cancelled paid order")};assertCounts(o.Order.ID(),1,1,1,1)}else{requireDomain(t,errs[0],domain.Conflict,"order_closed");if r.Order.Status()!=domain.Cancelled || r.Order.Paid()!=0{t.Fatal("payment survived cancellation")};assertCounts(o.Order.ID(),0,0,2,2)}
		}
	})
	t.Run("payment_versus_pickup_and_terminal_replay",func(t *testing.T){
		o:=create("pickup-create")
		if _,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"pickup-process");err!=nil{t.Fatal(err)}
		if _,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Ready,2,"pickup-ready");err!=nil{t.Fatal(err)}
		in:=PaymentInput{OrderID:o.Order.ID(),IdempotencyKey:"pickup-payment",Amount:10000,Method:domain.Cash}
		errs:=concurrentOrderCommands(2,func(i int)error{if i==0{_,err:=payments.Record(ctx,a.ID,in);return err};_,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Collected,3,"pickup-attempt");return err})
		if errs[0]!=nil || errs[1]==nil{t.Fatalf("unpaid/stale pickup accepted: %v",errs)}
		requireDomain(t,errs[1],domain.Conflict,"")
		r:=read(o.Order.ID());if r.Version!=4 || r.Order.Status()!=domain.Ready || r.Order.PaymentStatus()!=domain.Paid{t.Fatal("race corrupted balance/status")}
		collected,err:=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Collected,4,"pickup-confirmed");if err!=nil{t.Fatal(err)}
		if collected.Version!=5 || collected.Order.CollectedAt().IsZero(){t.Fatal("pickup milestone missing")}
		replay,err:=payments.Record(ctx,a.ID,in);if err!=nil || !replay.Replayed{t.Fatal("terminal payment replay failed",err)}
		in.IdempotencyKey="after-pickup";in.Amount=1;_,err=payments.Record(ctx,a.ID,in);requireDomain(t,err,domain.Conflict,"order_closed")
		assertCounts(o.Order.ID(),1,1,4,4)
	})
	t.Run("late_failures_rollback_status_history_receipt_and_audit",func(t *testing.T){
		for _,stage:=range []string{"audit","history","receipt"}{
			o:=create("rollback-create-"+stage)
			_,err:=NewOrders(workflowFailureWork{UnitOfWork:base,stage:stage},rt).ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"rollback-"+stage)
			if !errors.Is(err,failure){t.Fatal("failure not propagated",stage,err)}
			r:=read(o.Order.ID());if r.Version!=1 || r.Order.Status()!=domain.Received || !r.Order.ProcessingAt().IsZero(){t.Fatal("partial progress survived")}
			assertCounts(o.Order.ID(),0,0,1,1)
			if count(`SELECT count(*) FROM audit_events WHERE entity_id=$1 AND action='order_status_changed'`,o.Order.ID())!=0{t.Fatal("audit survived rollback")}
			if _,err=orders.ChangeStatus(ctx,a.ID,o.Order.ID(),domain.Processing,1,"rollback-"+stage);err!=nil{t.Fatal("key poisoned by rollback",err)}
		}
	})
	t.Run("waiting_duplicate_retries_after_first_transaction_rolls_back",func(t *testing.T){
		in:=input("rollback-duplicate-create");acquired,release:=make(chan struct{}),make(chan struct{})
		firstDone:=make(chan error,1);secondDone:=make(chan error,1)
		go func(){_,err:=NewOrders(workflowFailureWork{UnitOfWork:base,stage:"receipt",acquired:acquired,release:release},rt).Create(ctx,a.ID,in);firstDone<-err}()
		select{case <-acquired:case <-ctx.Done():t.Fatal(ctx.Err())}
		go func(){_,err:=orders.Create(ctx,a.ID,in);secondDone<-err}()
		// Observe the lock wait in PostgreSQL rather than relying on scheduler sleep.
		deadline:=time.Now().Add(5*time.Second);blocked:=false
		for time.Now().Before(deadline){if err:=db.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock')`).Scan(&blocked);err!=nil{close(release);t.Fatal(err)};if blocked{break};time.Sleep(5*time.Millisecond)}
		close(release)
		if !errors.Is(<-firstDone,failure){t.Fatal("first command did not roll back")};if err:=<-secondDone;err!=nil{t.Fatal("waiting duplicate failed",err)};if !blocked{t.Fatal("duplicate never waited for key lock")}
		if count(`SELECT count(*) FROM order_command_receipts WHERE idempotency_key=$1`,in.IdempotencyKey)!=1{t.Fatal("duplicate receipt")}
	})
	t.Run("database_rejects_invalid_graph_and_history_tampering",func(t *testing.T){
		o:=create("constraint-create")
		for _,statement:=range []string{
			`UPDATE orders SET status='ready',ready_at=updated_at,version=version+1 WHERE id=$1`,
			`UPDATE orders SET status='processing',version=version+1 WHERE id=$1`,
			`UPDATE orders SET processing_at=updated_at,version=version+1 WHERE id=$1`,
			`UPDATE order_status_events SET to_status='cancelled' WHERE order_id=$1`,
			`DELETE FROM order_status_events WHERE order_id=$1`,
			`DELETE FROM order_command_receipts WHERE order_id=$1`,
		}{_,err:=db.ExecContext(ctx,statement,o.Order.ID());var pg *pq.Error;if !errors.As(err,&pg)||pg.Code!="23514"{t.Fatal("constraint/append-only not enforced",statement,err)}}
		assertCounts(o.Order.ID(),0,0,1,1)
	})
}

var _ *sql.DB // SQL-backed test fixtures are intentionally separate from fakes.
