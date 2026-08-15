// Command seed mints a tenant and an API key for local/dev use. It prints the plaintext
// key ONCE and stores only its hash - the same hash otp-api computes when authenticating
// an incoming key (via pkg/security). Run against the compose MySQL:
//
//	go run ./services/seed --name demo
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"

	"github.com/duykhanh/worklane/pkg/platform/config"
	"github.com/duykhanh/worklane/pkg/platform/mysql"
	"github.com/duykhanh/worklane/pkg/security"
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	name := flag.String("name", "", "tenant name (required)")
	flag.Parse()
	if *name == "" {
		log.Fatal("seed: --name is required")
	}

	dsn := config.Env("MYSQL_DSN", "root:secret@tcp(localhost:3306)/otp?parseTime=true&multiStatements=true")
	db, err := mysql.Open(dsn)
	if err != nil {
		log.Fatalf("seed: mysql: %v", err)
	}

	key, err := security.GenerateAPIKey()
	if err != nil {
		log.Fatalf("seed: generate key: %v", err)
	}
	tenantID, keyID := newID(), newID()

	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, *name).Error; err != nil {
		log.Fatalf("seed: insert tenant: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO api_keys (id, tenant_id, hashed_key, status) VALUES (?, ?, ?, 'active')",
		keyID, tenantID, security.HashKey(key),
	).Error; err != nil {
		log.Fatalf("seed: insert api key: %v", err)
	}

	fmt.Printf("tenant_id: %s\n", tenantID)
	fmt.Printf("API key (shown once, store it now):\n\n    %s\n\n", key)
}
