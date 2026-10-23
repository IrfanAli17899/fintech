package catalog

import (
	_ "example.com/shop/catalog/internal/adapter"
	_ "example.com/shop/catalog/internal/persistence"
	_ "example.com/shop/catalog/internal/transport"
	_ "example.com/shop/platform/database"
)
