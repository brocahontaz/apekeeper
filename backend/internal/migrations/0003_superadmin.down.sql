UPDATE users SET app_role='admin' WHERE app_role='superadmin';
ALTER TABLE users DROP CONSTRAINT users_app_role_check;
ALTER TABLE users ADD CONSTRAINT users_app_role_check CHECK (app_role IN ('admin','officer','member'));
