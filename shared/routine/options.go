package routine

import (
	"fmt"
	"time"
)

// Option changes how a routine runs. Each applies to some kinds only; given to
// another, it panics at registration.
type Option struct {
	name  string
	kinds []kind
	apply func(*options)
}

type options struct {
	immediately            bool
	jitter                 float64
	backoffMin, backoffMax time.Duration
	recover                bool
}

func newOptions(k kind, routine string, opts []Option) options {
	o := options{jitter: 0.1, backoffMin: time.Second, backoffMax: 30 * time.Second}
	for _, opt := range opts {
		ok := false
		for _, allowed := range opt.kinds {
			ok = ok || allowed == k
		}
		if !ok {
			panic(fmt.Sprintf("routine: %s: %s does not apply to %s", routine, opt.name, k))
		}
		opt.apply(&o)
	}
	return o
}

// Immediately runs an Every's first run at Start, rather than a random part of
// its interval later.
func Immediately() Option {
	return Option{name: "Immediately", kinds: []kind{kindEvery}, apply: func(o *options) { o.immediately = true }}
}

// Jitter sets the jitter on an Every's interval: each wait is the interval,
// give or take f of it. 0.1 by default; 0 for none.
func Jitter(f float64) Option {
	if f < 0 || f >= 1 {
		panic(fmt.Sprintf("routine: Jitter(%v) is outside [0, 1)", f))
	}
	return Option{name: "Jitter", kinds: []kind{kindEvery}, apply: func(o *options) { o.jitter = f }}
}

// RestartBackoff sets a Loop's restart delay: exponential from min to max, with
// jitter, and reset once a run lasts longer than max. 1s to 30s by default.
func RestartBackoff(min, max time.Duration) Option {
	checkBackoff(min, max)
	return Option{name: "RestartBackoff", kinds: []kind{kindLoop}, apply: func(o *options) { o.backoffMin, o.backoffMax = min, max }}
}

// Recover makes an Every recover a panic in a run, counting it as a failed run
// and carrying on, rather than crashing the process. Use it only for a routine
// whose run holds no state outside its own call: one that works through a
// database transaction, which a panic rolls back, and touches no shared memory.
func Recover() Option {
	return Option{name: "Recover", kinds: []kind{kindEvery}, apply: func(o *options) { o.recover = true }}
}
