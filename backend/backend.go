package backend

import (
	"net/url"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // per gorm
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

	if !db.HasTable(&TokenData{}) {
		db.CreateTable(&TokenData{})
	}

	return &Backend{
		db: db,
	}, nil
}

// Close wraps the db close function for easy cleanup
func (b *Backend) Close() {
	b.db.Close()
}
