-- Additive catalog metadata and history. Accepted order snapshots stay untouched.
ALTER TABLE public.services ADD COLUMN display_order INTEGER NOT NULL DEFAULT 0 CHECK(display_order BETWEEN 0 AND 1000000);
CREATE INDEX services_display_idx ON public.services(display_order,name,id);
CREATE TABLE public.service_revisions (
    service_id VARCHAR(100) NOT NULL REFERENCES public.services(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL CHECK(version>0),
    name VARCHAR(100) NOT NULL CHECK(length(btrim(name))>0),
    unit VARCHAR(10) NOT NULL CHECK(unit IN('kg','piece')),
    unit_price BIGINT NOT NULL CHECK(unit_price>=0),
    turnaround_ns BIGINT NOT NULL CHECK(turnaround_ns>0),
    active BOOLEAN NOT NULL,
    display_order INTEGER NOT NULL CHECK(display_order BETWEEN 0 AND 1000000),
    changed_by VARCHAR(100) REFERENCES public.staff(id) ON DELETE RESTRICT,
    changed_at TIMESTAMPTZ NOT NULL,
    source VARCHAR(10) NOT NULL CHECK(source IN('baseline','staff','demo')),
    PRIMARY KEY(service_id,version),
    CHECK((source='staff' AND changed_by IS NOT NULL) OR (source IN('baseline','demo') AND changed_by IS NULL))
);
-- Baseline means first observed state, not a reconstruction of past prices.
INSERT INTO public.service_revisions(service_id,version,name,unit,unit_price,turnaround_ns,active,display_order,changed_at,source)
SELECT id,version,name,unit,unit_price,turnaround_ns,active,display_order,updated_at,'baseline' FROM public.services;
CREATE TRIGGER service_revisions_append_only BEFORE UPDATE OR DELETE ON public.service_revisions
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();
