-- Additive customer management. No merge, deletion or legacy conversion.
ALTER TABLE public.customers ADD COLUMN address VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE public.customers ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
-- Existing capabilities get a bounded grace period from migration application.
ALTER TABLE public.order_tracking_tokens ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '90 days');
ALTER TABLE public.order_tracking_tokens ALTER COLUMN expires_at DROP DEFAULT;
ALTER TABLE public.order_tracking_tokens ADD CONSTRAINT tracking_expiry_finite CHECK(isfinite(expires_at));
ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
 (entity='order' AND action IN('order_created','order_status_changed','order_cancelled','discount_approved','order_tracking_rotated'))
 OR(entity='payment' AND action='payment_recorded') OR(entity='customer' AND action IN('customer_created','customer_updated'))
 OR(entity='service' AND action IN('service_configured','service_updated')) OR(entity='report' AND action='report_generated')
 OR(entity='staff' AND action IN('staff_updated','staff_logged_in','staff_created','staff_bootstrapped','staff_deactivated','staff_activated','staff_role_changed','staff_password_reset'))
);
