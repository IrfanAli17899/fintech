package persistence

import (
	_ "database/sql"
	_ "example.com/shop/catalog/internal/domain"
)

var q = "SELECT id FROM catalog.products"
