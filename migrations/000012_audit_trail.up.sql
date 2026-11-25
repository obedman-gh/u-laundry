-- Existing rows remain unchanged: historical request IDs cannot be reconstructed.
ALTER TABLE public.audit_events ADD COLUMN request_id VARCHAR(100);
ALTER TABLE public.audit_events ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}';
ALTER TABLE public.audit_events ADD CONSTRAINT audit_request_id_check
 CHECK(request_id IS NULL OR request_id ~ '^[A-Za-z0-9_.-]{1,100}$');

-- Typed, bounded metadata only. Never accept generic request/response snapshots.
CREATE FUNCTION public.ulaundry_safe_audit_metadata(m JSONB) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog AS $$
DECLARE k TEXT; v JSONB;
BEGIN
 IF jsonb_typeof(m) <> 'object' THEN RETURN false; END IF;
 FOR k,v IN SELECT * FROM jsonb_each(m) LOOP
  CASE k
   WHEN 'version' THEN
    IF jsonb_typeof(v)<>'number' OR v::text !~ '^[1-9][0-9]*$' OR (v::text)::numeric>9223372036854775807 THEN RETURN false; END IF;
   WHEN 'unit_price' THEN
    IF jsonb_typeof(v)<>'number' OR v::text !~ '^(0|[1-9][0-9]*)$' OR (v::text)::numeric>9223372036854775807 THEN RETURN false; END IF;
   WHEN 'active' THEN IF jsonb_typeof(v)<>'boolean' THEN RETURN false; END IF;
   WHEN 'status' THEN IF v NOT IN ('"received"','"processing"','"ready"','"collected"','"cancelled"') THEN RETURN false; END IF;
   WHEN 'role' THEN IF v NOT IN ('"owner"','"admin"','"cashier"') THEN RETURN false; END IF;
   ELSE RETURN false;
  END CASE;
 END LOOP;
 RETURN true;
END $$;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_metadata_check
 CHECK(public.ulaundry_safe_audit_metadata(metadata));

CREATE FUNCTION public.ulaundry_audit_request_required() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.request_id IS NULL OR NEW.request_id='' THEN
  RAISE EXCEPTION 'New audit events require request correlation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_events_request_required BEFORE INSERT ON public.audit_events
 FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_request_required();

-- Preserve all existing allowed actions, including historical login markers.
ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
 (entity='order' AND action IN('order_created','order_status_changed','order_cancelled','discount_approved','order_tracking_rotated'))
 OR(entity='payment' AND action='payment_recorded') OR(entity='refund' AND action='payment_refunded')
 OR(entity='customer' AND action IN('customer_created','customer_updated'))
 OR(entity='service' AND action IN('service_configured','service_updated')) OR(entity='report' AND action='report_generated')
 OR(entity='staff' AND action IN('staff_updated','staff_logged_in','credentials_verified','staff_created','staff_bootstrapped','staff_deactivated','staff_activated','staff_role_changed','staff_password_reset'))
 OR(entity='shift' AND action IN('shift_opened','shift_closed','shift_cash_moved'))
);
CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON public.audit_events
 FOR EACH STATEMENT EXECUTE FUNCTION public.ulaundry_audit_append_only();
CREATE TRIGGER auth_events_no_truncate BEFORE TRUNCATE ON public.auth_events
 FOR EACH STATEMENT EXECUTE FUNCTION public.ulaundry_audit_append_only();
CREATE INDEX audit_time_idx ON public.audit_events(occurred_at DESC,id DESC);
CREATE INDEX audit_action_time_idx ON public.audit_events(action,occurred_at DESC,id DESC);
CREATE INDEX audit_request_idx ON public.audit_events(request_id) WHERE request_id IS NOT NULL;
CREATE INDEX auth_action_time_idx ON public.auth_events(kind,occurred_at DESC,id DESC);
CREATE INDEX auth_request_idx ON public.auth_events(request_id);
