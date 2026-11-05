package godeps_test

import "testing"

func TestJSONPrintsClassesAndTheImportsBetweenThem(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":                     goMod,
		"web/server.go":              "package web\n\nimport \"example.com/shop/billing/invoice\"\n\nvar _ = invoice.Total\n",
		"billing/billing.go":         "package billing\n",
		"billing/invoice/invoice.go": "package invoice\n\nvar Total = 1\n",
	})
	stdout, stderr := run(t, dir, 0, "--format", "json")
	want := `{
  "version": 1,
  "title": "example.com/shop",
  "module": "example.com/shop",
  "packages": [
    {
      "id": "billing",
      "name": "billing",
      "packageCount": 2
    },
    {
      "id": "billing/invoice",
      "name": "invoice",
      "parent": "billing",
      "packageCount": 1
    },
    {
      "id": "web",
      "name": "web",
      "packageCount": 1
    }
  ],
  "classes": [
    {
      "id": "example.com/shop/billing",
      "name": "billing",
      "path": "billing",
      "package": "billing"
    },
    {
      "id": "example.com/shop/billing/invoice",
      "name": "invoice",
      "path": "billing/invoice",
      "package": "billing/invoice"
    },
    {
      "id": "example.com/shop/web",
      "name": "web",
      "path": "web",
      "package": "web"
    }
  ],
  "edges": [
    {
      "from": "example.com/shop/web",
      "to": "example.com/shop/billing/invoice",
      "kind": "dependency"
    }
  ]
}
`
	if stdout != want {
		t.Errorf("stdout = %s\nwant %s", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}
