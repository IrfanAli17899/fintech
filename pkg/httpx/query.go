package httpx

import (
	"net/http"
	"reflect"
	"strconv"

	"github.com/irfanali17899/fintech/pkg/errdefs"
)

// DecodeQuery fills a struct from URL query params using `query:"name"` tags.
// Supports string, int, int64, bool. Missing params leave the zero value;
// required-ness is the caller's Validate() concern.
func DecodeQuery[T any](r *http.Request) (T, error) {
	var out T
	q := r.URL.Query()

	v := reflect.ValueOf(&out).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Tag.Get("query")
		if name == "" {
			continue
		}
		raw := q.Get(name)
		if raw == "" {
			continue
		}

		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(raw)
		case reflect.Int, reflect.Int64:
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return out, errdefs.Validationf("INVALID_QUERY_PARAM", "%s must be an integer", name)
			}
			f.SetInt(n)
		case reflect.Bool:
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return out, errdefs.Validationf("INVALID_QUERY_PARAM", "%s must be true or false", name)
			}
			f.SetBool(b)
		}
	}
	return out, nil
}
