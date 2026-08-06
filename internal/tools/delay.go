package tools

import (
	"context"
	"time"
	"unicode/utf8"
)

// TypingDelay returns how long it takes to "type" text at the given speed
// (characters per second). Runes are counted, not bytes, so multibyte
// alphabets (e.g. Cyrillic) are typed proportionally.
func TypingDelay(text string, speed float64) time.Duration {
	if speed <= 0 || text == "" {
		return 0
	}
	return time.Duration(float64(utf8.RuneCountInString(text)) / speed * float64(time.Second))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d Deps) sleep(ctx context.Context, dur time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, dur)
	}
	return sleepCtx(ctx, dur)
}

func (d Deps) schedule(ctx context.Context, delay time.Duration, task func(ctx context.Context)) error {
	if d.Schedule != nil {
		return d.Schedule(ctx, delay, task)
	}
	time.AfterFunc(delay, func() { task(context.Background()) })
	return nil
}
