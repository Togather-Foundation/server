-- Historical errors cannot be classified; leave the new counters at zero.
ALTER TABLE api_key_usage ADD COLUMN client_error_count BIGINT NOT NULL DEFAULT 0, ADD COLUMN server_error_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE api_key_usage_ips ADD COLUMN client_error_count BIGINT NOT NULL DEFAULT 0, ADD COLUMN server_error_count BIGINT NOT NULL DEFAULT 0;
