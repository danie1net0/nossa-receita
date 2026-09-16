package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"codeberg.org/cuducos/minha-receita/db"
)

type DatabaseArgs struct {
	URI            string
	PostgresSchema string
	PostgresLogged bool
}

type database interface {
	Create() error
	Drop() error
	Close()

	// transform
	PreLoad() error
	CreateCompanies(context.Context, [][]string) error
	CopyRelational(context.Context, string, []string, [][]any) error
	PostLoad() error
	MetaSave(string, string) error

	// extra indexes
	CreateExtraIndexes(idxs []string) error

	// api
	GetCompany(context.Context, string) (string, error)
	Search(context.Context, *db.Query) (string, error)
	MetaRead(string) (string, error)
	AllCompanies(context.Context, func(string)) error

	// graph
	CreateGraphTable() error
	GetRelated(context.Context, string) ([]db.GraphEdge, error)
}

func loadPostgres(args *DatabaseArgs) (*db.PostgreSQL, error) {
	if args.URI == "" {
		args.URI = os.Getenv("DATABASE_URL")
	}
	if args.URI == "" {
		return nil, fmt.Errorf("could not find a database URI, set the DATABASE_URL environment variable with the credentials for a database")
	}
	if !strings.HasPrefix(args.URI, "postgres://") && !strings.HasPrefix(args.URI, "postgresql://") {
		return nil, fmt.Errorf("database uri does not seem to be a valid PostgreSQL URI")
	}
	pg, err := db.NewPostgreSQL(args.URI, args.PostgresSchema, args.PostgresLogged)
	if err != nil {
		return nil, err
	}
	return &pg, nil
}

func loadDatabase(args *DatabaseArgs) (database, error) {
	pg, err := loadPostgres(args)
	if err != nil {
		return nil, err
	}
	return pg, nil
}
