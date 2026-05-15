-- Run as a PostgreSQL superuser for local development:
--   sudo -u postgres psql -f scripts/setup_postgres.sql
--
-- App DATABASE_URL:
--   postgres://statuspage:statuspage@localhost:5432/statuspage?sslmode=disable

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'statuspage') THEN
    CREATE ROLE statuspage LOGIN PASSWORD 'statuspage';
  END IF;
END
$$;

SELECT 'CREATE DATABASE statuspage OWNER statuspage'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'statuspage')\gexec

GRANT ALL PRIVILEGES ON DATABASE statuspage TO statuspage;
