package bash

import (
	"context"
	"testing"
)

func TestRunCat(t *testing.T) {
	ctx := context.Background()
	_, err := Run(ctx, "/Users/mac/study/mini-gnet", "cat ./tools/bash/bash_test.go")
	if err != nil {
		t.Errorf("Run failed: %v", err)
	}

	// t.Logf("Run returned: %v", s1)
}

func TestRunRm(t *testing.T) {
	ctx := context.Background()
	_, err := Run(ctx, "/Users/mac/study/mini-gnet", "rm ./tools/bash/bash_test.go")
	if err == nil {
		t.Errorf("Run failed: %v", err)
	}

	t.Logf("Run returned: %v", err)
}
