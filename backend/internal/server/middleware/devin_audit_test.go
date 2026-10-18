package middleware

import "testing"

func TestDevinAuditOmissionAndSensitiveReadCoverage(t *testing.T) {
	for _, route := range []string{
		"POST /api/v1/admin/accounts/import/devin",
		"POST /api/v1/admin/devin/merged-response",
		"POST /api/v1/admin/devin/models/probe",
		"POST /api/v1/admin/devin/host-keys",
	} {
		if _, ok := auditBodyOmittedRoutes[route]; !ok {
			t.Fatalf("sensitive request body not omitted: %s", route)
		}
	}
	for _, route := range []string{
		"GET /api/v1/admin/devin/requests/export",
		"GET /api/v1/admin/devin/requests/:id/export",
		"GET /api/v1/admin/devin/requests/:id/files/*file",
		"GET /api/v1/admin/devin/host-keys",
		"GET /api/v1/admin/devin/profiles/:profile",
	} {
		if auditSensitiveReads[route] == "" {
			t.Fatalf("sensitive read missing audit: %s", route)
		}
	}
}
