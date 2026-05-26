package runtimetypes

// LocalTenantID is the single-tenant identity used by the OSS runtime. It is
// passed explicitly to every tenant-scoped API (vfsservice, vfsstore,
// runtimestate.EnsureModels) so that proprietary builds layering multi-tenancy
// on top can substitute real tenant values without forking the OSS.
const LocalTenantID = "00000000-0000-0000-0000-000000000001"
