package main

import (
	"sync"
	"testing"
)

// TestApplicationAccountsConcurrentAccess is a regression test for
// unsynchronized access to the accounts map, which was read by every
// authenticated request and written by login/logout.
func TestApplicationAccountsConcurrentAccess(t *testing.T) {
	app := &Application{accounts: make(map[string]*MigakuClient)}

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for j := range 200 {
				key := string(rune('a' + j%26))
				app.setAccount(key, &MigakuClient{})
				app.getAccount(key)
				app.listAccounts()
				app.deleteAccount(key)
			}
		})
	}
	wg.Wait()

	client := &MigakuClient{}
	app.setAccount("final", client)
	got, ok := app.getAccount("final")
	if !ok || got != client {
		t.Error("expected the registered account to be returned")
	}
}

func TestApplicationAccountHelpers(t *testing.T) {
	app := &Application{accounts: make(map[string]*MigakuClient)}

	if _, ok := app.getAccount("missing"); ok {
		t.Fatal("expected no account for unknown key")
	}

	client := &MigakuClient{}
	app.setAccount("key", client)

	got, ok := app.getAccount("key")
	if !ok || got != client {
		t.Fatal("expected registered account to be returned")
	}

	if len(app.listAccounts()) != 1 {
		t.Fatalf("expected 1 account, got %d", len(app.listAccounts()))
	}

	app.deleteAccount("key")

	if _, ok := app.getAccount("key"); ok {
		t.Fatal("expected account to be deleted")
	}
}
