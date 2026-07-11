package health

import (
	"context"
	"fmt"
)

type CheckFunc func(ctx context.Context) error

type Checker struct {
	checks map[string]CheckFunc
}

func NewChecker(checks map[string]CheckFunc) *Checker {
	if checks == nil {
		checks = map[string]CheckFunc{}
	}

	return &Checker{checks: checks}
}

func (c *Checker) Check(ctx context.Context) (bool, map[string]string) {
	results := make(map[string]string, len(c.checks))
	ready := true

	for name, check := range c.checks {
		if err := check(ctx); err != nil {
			ready = false
			results[name] = err.Error()
			continue
		}

		results[name] = "ok"
	}

	return ready, results
}

func Ping(name string, fn CheckFunc) (string, CheckFunc) {
	return name, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		return nil
	}
}
