package domain_test

import (
	"testing"
	"time"
	"ulaundry/internal/domain"
)

func TestOrderProcessingMilestoneAndLegacyHydration(t *testing.T){
	o:=order(t)
	if !o.ProcessingAt().IsZero(){t.Fatal("processing before acceptance")}
	if err:=o.Transition(domain.Processing,now.Add(time.Minute),"");err!=nil{t.Fatal(err)}
	if !o.ProcessingAt().Equal(now.Add(time.Minute)){t.Fatal("missing processing milestone")}
	if err:=o.Transition(domain.Ready,now.Add(2*time.Minute),"");err!=nil{t.Fatal(err)}
	s:=o.Snapshot()
	for _,bad:=range []time.Time{now.Add(-time.Second),now.Add(3*time.Minute)}{
		s.ProcessingAt=bad;_,err:=domain.RestoreOrder(s);requireError(t,err,domain.Validation,"")
	}
	s.ProcessingAt=time.Time{}
	legacy,err:=domain.RestoreOrder(s)
	if err!=nil || !legacy.ProcessingAt().IsZero(){t.Fatal("historical processing time fabricated",err)}
	s=order(t).Snapshot();s.ProcessingAt=now
	_,err=domain.RestoreOrder(s);requireError(t,err,domain.Validation,"")
}
