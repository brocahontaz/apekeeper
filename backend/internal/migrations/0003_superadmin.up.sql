ALTER TABLE users DROP CONSTRAINT users_app_role_check;
ALTER TABLE users ADD CONSTRAINT users_app_role_check CHECK (app_role IN ('superadmin','admin','officer','member'));
