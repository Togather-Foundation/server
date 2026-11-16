ALTER TABLE api_key_usage_ips DROP COLUMN client_error_count, DROP COLUMN server_error_count;
ALTER TABLE api_key_usage DROP COLUMN client_error_count, DROP COLUMN server_error_count;
