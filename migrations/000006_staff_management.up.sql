-- Additive staff administration; preserve existing accounts and credentials.
ALTER TABLE public.staff ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
 (entity='order' AND action IN('order_created','order_status_changed','order_cancelled','discount_approved'))
 OR(entity='payment' AND action='payment_recorded') OR(entity='customer' AND action IN('customer_created','customer_updated'))
 OR(entity='service' AND action IN('service_configured','service_updated')) OR(entity='report' AND action='report_generated')
 OR(entity='staff' AND action IN('staff_updated','staff_logged_in','staff_created','staff_bootstrapped','staff_deactivated','staff_activated','staff_role_changed','staff_password_reset'))
);
ALTER TABLE public.auth_events DROP CONSTRAINT auth_events_kind_check;
ALTER TABLE public.auth_events ADD CONSTRAINT auth_events_kind_check CHECK(kind IN('login_success','login_failure','login_limited','logout','access_denied'));
ALTER TABLE public.auth_events ADD COLUMN permission VARCHAR(40) NOT NULL DEFAULT '';
ALTER TABLE public.auth_events ADD CONSTRAINT auth_events_permission_check CHECK(
 (kind='access_denied' AND permission IN('manage_staff','read_audit','manage_configuration','approve_discount','create_customer','configure_service','create_order','change_status','record_payment','cancel_order','read_orders','generate_report'))
 OR(kind<>'access_denied' AND permission='')
);
