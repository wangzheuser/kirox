package task

import "testing"

func TestRegistrationAttemptCounterCountsOnlyStartedRegistrations(t *testing.T) {
	counter := newRegistrationAttemptCounter(2)

	// 前置阶段（邮箱创建/代理初始化前）失败不会调用 reserve，因此不消耗注册次数。
	first, ok := counter.reserve()
	if !ok || first != 0 {
		t.Fatalf("first reserve = (%d,%v), want (0,true)", first, ok)
	}
	second, ok := counter.reserve()
	if !ok || second != 1 {
		t.Fatalf("second reserve = (%d,%v), want (1,true)", second, ok)
	}
	third, ok := counter.reserve()
	if ok || third != 0 {
		t.Fatalf("third reserve = (%d,%v), want (0,false)", third, ok)
	}
	if !counter.done() {
		t.Fatalf("counter should be done after two started registrations")
	}
}
