package style

import (
	"reflect"
	"strings"
	"testing"
)

func TestGraphiteVioletDefinesEveryToken(t *testing.T) {
	v := reflect.ValueOf(GraphiteViolet)
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if got := string(v.Field(i).String()); strings.TrimSpace(got) == "" {
			t.Errorf("token %s is empty", name)
		}
	}
	if Pad < 1 || Gap < 1 {
		t.Errorf("spacing tokens must be positive: Pad=%d Gap=%d", Pad, Gap)
	}
}
