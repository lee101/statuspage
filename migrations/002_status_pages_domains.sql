ALTER TABLE status_pages
  ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS website_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS logo_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS theme_color TEXT NOT NULL DEFAULT '#34c759',
  ADD COLUMN IF NOT EXISTS directory_listed BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS published BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS status_page_domains (
  id BIGSERIAL PRIMARY KEY,
  status_page_id BIGINT NOT NULL REFERENCES status_pages(id) ON DELETE CASCADE,
  hostname TEXT NOT NULL UNIQUE,
  domain_type TEXT NOT NULL DEFAULT 'path',
  route_slug TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  dns_record_id TEXT,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_status_page_domains_slug ON status_page_domains(route_slug);
CREATE INDEX IF NOT EXISTS idx_status_pages_directory ON status_pages(directory_listed, published);
