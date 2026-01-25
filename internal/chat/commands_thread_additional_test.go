package chat

import (
	"testing"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestIsAncestorOf(t *testing.T) {
	dbConn := openChatDB(t)

	meta, err := db.CreateThread(dbConn, types.Thread{GUID: "thrd-meta", Name: "meta"})
	if err != nil {
		t.Fatalf("create meta: %v", err)
	}
	opus, err := db.CreateThread(dbConn, types.Thread{GUID: "thrd-opus", Name: "opus", ParentThread: &meta.GUID})
	if err != nil {
		t.Fatalf("create opus: %v", err)
	}
	_, err = db.CreateThread(dbConn, types.Thread{GUID: "thrd-notes", Name: "notes", ParentThread: &opus.GUID})
	if err != nil {
		t.Fatalf("create notes: %v", err)
	}

	m := &Model{db: dbConn}
	ok, err := m.isAncestorOf("thrd-notes", "thrd-meta")
	if err != nil {
		t.Fatalf("isAncestorOf: %v", err)
	}
	if !ok {
		t.Fatalf("expected meta to be ancestor")
	}

	ok, err = m.isAncestorOf("thrd-opus", "thrd-notes")
	if err != nil {
		t.Fatalf("isAncestorOf false: %v", err)
	}
	if ok {
		t.Fatalf("did not expect notes to be ancestor of opus")
	}
}

func TestCheckMetaPathCollision(t *testing.T) {
	dbConn := openChatDB(t)

	meta, err := db.CreateThread(dbConn, types.Thread{GUID: "thrd-meta", Name: "meta"})
	if err != nil {
		t.Fatalf("create meta: %v", err)
	}
	_, err = db.CreateThread(dbConn, types.Thread{GUID: "thrd-opus", Name: "opus", ParentThread: &meta.GUID})
	if err != nil {
		t.Fatalf("create opus: %v", err)
	}

	m := &Model{db: dbConn}
	if err := m.checkMetaPathCollision(nil, "opus"); err == nil {
		t.Fatalf("expected meta path collision error")
	}
}

func TestIsRoomDestination(t *testing.T) {
	m := &Model{projectName: "fray"}
	if !m.isRoomDestination("room") {
		t.Fatalf("expected room to be true")
	}
	if !m.isRoomDestination("main") {
		t.Fatalf("expected main to be true")
	}
	if !m.isRoomDestination("fray") {
		t.Fatalf("expected project name to be true")
	}
	if m.isRoomDestination("design") {
		t.Fatalf("expected design to be false")
	}
}
