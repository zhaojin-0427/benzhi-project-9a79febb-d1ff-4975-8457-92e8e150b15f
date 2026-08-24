package workflow

import (
	"fmt"
	"time"
)

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
