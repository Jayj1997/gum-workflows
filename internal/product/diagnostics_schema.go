package product

// ProductSchemaVersion is the SQLite product schema version the running
// Application builds against. It is carried into diagnostics bundles so a
// crash report names the persisted shape it was generated against. The
// authoritative value lives in internal/history; re-declaring it here keeps
// the Application seam from importing its persistence adapter.
const ProductSchemaVersion = 11
