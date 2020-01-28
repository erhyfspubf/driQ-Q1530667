package dscache

import (
	"context"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/qri-io/qri/logbook"
	"github.com/qri-io/qfs"
	"github.com/qri-io/qri/dsref"
	"github.com/qri-io/dataset"
)

// Builder builds a logbook in a convenient way
type Builder struct {
	Book       *logbook.Book
	AuthorName string
	Dsrefs     map[string][]string
}

// NewLogbookTempBuilder constructs a logbook tmp Builder
func NewLogbookTempBuilder(t *testing.T, privKey crypto.PrivKey, username string, fs qfs.Filesystem, rootPath string) Builder {
	book, err := logbook.NewJournal(privKey, username, fs, rootPath)
	if err != nil {
		t.Fatal(err)
	}
	Builder := Builder{
		Book:       book,
		AuthorName: username,
		Dsrefs:     make(map[string][]string),
	}
	return Builder
}

// DatasetInit initializes a new dataset and return a reference to it
func (b *Builder) DatasetInit(ctx context.Context, t *testing.T, dsname string) dsref.Ref {
	if err := b.Book.WriteDatasetInit(ctx, dsname); err != nil {
		t.Fatal(err)
	}
	b.Dsrefs[dsname] = make([]string, 0)
	return dsref.Ref{Username: b.AuthorName, Name: dsname}
}

// DatasetRename changes the name of a dataset
func (b *Builder) DatasetRename(ctx context.Context, t *testing.T, ref dsref.Ref, newName string) dsref.Ref {
	b.ensureAuthorAllowed(t, ref.Username)
	if err := b.Book.WriteDatasetRename(ctx, ref, newName); err != nil {
		t.Fatal(err)
	}
	b.Dsrefs[newName] = b.Dsrefs[ref.Name]
	delete(b.Dsrefs, ref.Name)
	return dsref.Ref{Username: b.AuthorName, Name: newName, Path: ref.Path}
}

// Commit adds a commit to a dataset
func (b *Builder) Commit(ctx context.Context, t *testing.T, ref dsref.Ref, title, ipfsHash string) dsref.Ref {
	b.ensureAuthorAllowed(t, ref.Username)
	ds := dataset.Dataset{
		Peername: ref.Username,
		Name:     ref.Name,
		Commit: &dataset.Commit{
			Timestamp: time.Unix(0, logbook.NewTimestamp()),
			Title:     title,
		},
		Path:         ipfsHash,
		PreviousPath: ref.Path,
	}
	if err := b.Book.WriteVersionSave(ctx, &ds); err != nil {
		t.Fatal(err)
	}
	b.Dsrefs[ref.Name] = append(b.Dsrefs[ref.Name], ipfsHash)
	return dsref.Ref{Username: ref.Username, Name: ref.Name, Path: ipfsHash}
}

// Delete removes some number of commits from a dataset
func (b *Builder) Delete(ctx context.Context, t *testing.T, ref dsref.Ref, num int) dsref.Ref {
	b.ensureAuthorAllowed(t, ref.Username)
	if err := b.Book.WriteVersionDelete(ctx, ref, num); err != nil {
		t.Fatal(err)
	}
	prevRefs := b.Dsrefs[ref.Name]
	nextRefs := prevRefs[:len(prevRefs)-num]
	b.Dsrefs[ref.Name] = nextRefs
	lastRef := nextRefs[len(nextRefs)-1]
	return dsref.Ref{Username: ref.Username, Name: ref.Name, Path: lastRef}
}

func (b *Builder) ensureAuthorAllowed(t *testing.T, peername string) {
	if peername != b.AuthorName {
		t.Fatalf("cannot rename dataset of %s, book owned by %s", peername, b.AuthorName)
	}
}

// Logbook returns the built logbook
func (b *Builder) Logbook() *logbook.Book {
	return b.Book
}
