package proxy

import "context"

type ctxKey int

const tenantCtxKey ctxKey = 1

func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey, tenantID)
}

func TenantFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(tenantCtxKey).(string)
	return v, ok && v != ""
}