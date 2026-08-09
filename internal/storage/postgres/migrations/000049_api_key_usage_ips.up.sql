CREATE TABLE api_key_usage_ips (
    api_key_id    UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    date          DATE NOT NULL,
    ip            INET NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    error_count   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (api_key_id, date, ip)
);

CREATE INDEX idx_api_key_usage_ips_date ON api_key_usage_ips (date);
CREATE INDEX idx_api_key_usage_ips_ip ON api_key_usage_ips (ip);
