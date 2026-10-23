-- Bind API keys to federation nodes.
--
-- Inbound federation sync must trust the authenticated principal, not the
-- payload @id host. A key bound to a node may only submit events minted by
-- that node. Unbound keys are rejected by the sync endpoint (fail closed).
ALTER TABLE api_keys
  ADD COLUMN federation_node_id UUID REFERENCES federation_nodes(id) ON DELETE SET NULL;

CREATE INDEX idx_api_keys_federation_node
  ON api_keys (federation_node_id)
  WHERE federation_node_id IS NOT NULL;
