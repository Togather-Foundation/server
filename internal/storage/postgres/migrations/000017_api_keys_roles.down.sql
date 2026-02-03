ALTER TABLE api_keys
  DROP CONSTRAINT api_keys_role_check;

ALTER TABLE api_keys
  ADD CONSTRAINT api_keys_role_check
  CHECK (role IN ('agent'));
