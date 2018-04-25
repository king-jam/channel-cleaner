package backend

import (
	"errors"
	"net/url"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // per gorm
)

var (
	// ErrRecordNotFound record not found error, happens when haven't find any matched data when looking up with a struct
	ErrRecordNotFound = errors.New("record not found")
)

// Backend stores all the Database internals for data access
type Backend struct {
	db *gorm.DB
}

// InitDatabase takes a connection string URL to pass into the Database
func InitDatabase(url *url.URL) (*Backend, error) {
	db, err := gorm.Open(url.Scheme, url.String())
	if err != nil {
		return nil, err
	}
	// SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
	db.DB().SetMaxIdleConns(20)

	// SetMaxOpenConns sets the maximum number of open connections to the database.
	db.DB().SetMaxOpenConns(20)
	return &Backend{
		db: db,
	}, nil
}

// Close wraps the db close function for easy cleanup
func (b *Backend) Close() {
	b.db.Close()
}
