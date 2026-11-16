-- Geography distance predicates cannot use the existing geometry-only GiST index.
CREATE INDEX idx_places_geography ON places USING GIST ((geo_point::geography));
