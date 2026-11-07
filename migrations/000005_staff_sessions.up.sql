-- Additive authentication storage; legacy and business history stay unchanged.
CREATE TABLE public.staff_sessions (
 token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash)=32),
 staff_id VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
 username VARCHAR(50) NOT NULL,
 role VARCHAR(10) NOT NULL CHECK (role IN ('owner','admin','cashier')),
 credential_fingerprint BYTEA NOT NULL CHECK (octet_length(credential_fingerprint)=32),
 issued_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at>issued_at AND expires_at<=issued_at+interval '12 hours'),
 revoked_at TIMESTAMPTZ
);
CREATE INDEX staff_sessions_staff_idx ON public.staff_sessions(staff_id);
CREATE INDEX staff_sessions_expiry_idx ON public.staff_sessions(expires_at);
-- Failed/unknown users have NULL actor, never a fabricated staff identity.
CREATE TABLE public.auth_events (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 kind VARCHAR(20) NOT NULL CHECK (kind IN ('login_success','login_failure','login_limited','logout')),
 actor_id VARCHAR(100) REFERENCES public.staff(id) ON DELETE RESTRICT,
 request_id VARCHAR(100) NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX auth_events_time_idx ON public.auth_events(occurred_at,id);
CREATE TRIGGER auth_events_append_only BEFORE UPDATE OR DELETE ON public.auth_events
 FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();

-- Credential/role/active/username changes revoke prior sessions permanently.
CREATE FUNCTION public.ulaundry_revoke_staff_sessions() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.staff_sessions SET revoked_at=clock_timestamp() WHERE staff_id=NEW.id AND revoked_at IS NULL;
 RETURN NEW;
END $$;
CREATE TRIGGER staff_authentication_changed AFTER UPDATE OF password_hash,role,active,username ON public.staff
 FOR EACH ROW WHEN (OLD.password_hash IS DISTINCT FROM NEW.password_hash OR OLD.role IS DISTINCT FROM NEW.role
 OR OLD.active IS DISTINCT FROM NEW.active OR OLD.username IS DISTINCT FROM NEW.username)
 EXECUTE FUNCTION public.ulaundry_revoke_staff_sessions();
