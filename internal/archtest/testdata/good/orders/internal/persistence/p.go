package persistence

import (
	_ "database/sql"
)

var q = "INSERT INTO orders.lines (id) VALUES (1)"
