package utils

import (
	"github.com/k0kubun/pp"
)

func PrettyPrint(i interface{}) string {
	return pp.Sprint(i)
}
