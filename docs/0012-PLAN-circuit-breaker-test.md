---
status: proposed
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Fix the Circuit-Breaker Test Race

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(revision 4, which gives the root cause and the decision).

This plan changes one test, `TestCircuitBreaker_IgnoresContextCancellation`
in `backplane_test.go`, and nothing else. It runs before the five 0012 plans,
so that the test cannot hang the gate while they run. If a fact contradicts
the MADR or this plan, **stop and prompt**, and record a dated entry in §9.

## Goal

The test cannot hang, and it still proves what it claims: a caller's context
cancellation never counts toward the circuit breaker.

## Scope

**In scope:** the one test function (Appendix B).

**Out of scope:** `backplane.go`, which is correct, and every other test.

## 0. Preconditions

* `main` at `5d05cf5` or a descendant with no change to `backplane_test.go`.
* The gate is the one every 0012 plan uses:
  * `gofmt -l` and `golint -set_exit_status` on `backplane_test.go`;
  * `go vet ./...` and `go vet -tags live_gateways ./llmprovider`;
  * `make lint`;
  * `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

## Phase B1 — Release the handler and remove the timer race

1. Apply Appendix B with `git apply`.
2. Run the gate.
3. Re-run the three mutants of Appendix A.2 on scratch copies. Each must fail
   the test; a hang counts, caught by `-timeout 30s`.
4. Run the stress command of Appendix A.3.
5. `git commit --no-edit`. No push and no tag.

**Changes:**
* The handler returns on `r.Context().Done()` or on `release`. A deferred
  `close(release)`, registered after `defer srv.Close()`, runs first.
* The five iterations use an already-cancelled context.
* One iteration's 50 ms deadline fires while the server holds the request.
* The assertions are unchanged.

## 7. Acceptance criteria

* The gate passes.
* All three mutants fail the test.
* The stress run passes with no hang.

## 8. Rollout and rollback

Test-only. Revert the commit to roll back.

## 9. Deviation log

None yet.

## 10. Execution record

Not executed. Appendix A was proven in scratch copies of `5d05cf5`.

## Appendix A — Proof record (2026-09-27, scratch copies of `5d05cf5`)

### A.1 Root cause reproduced

The reproduction used a deadline that fires after the request is sent,
standing in for a timer callback delivered late. For each handler, the test
checked whether `srv.Close()` returned within 5 s (rows in the order listed below):

| Reproduction test | Result | Time |
|---|---|---|
| `TestRepro1_SelectForeverHangsClose` | still blocked after 5 s | 5.00 s |
| `TestRepro2_ContextOnlyStillHangs` | still blocked after 5 s | 5.00 s |
| `TestRepro3_DrainThenContextCloses` | returned | 0.05 s |
| `TestRepro4_ReleaseCloses` | returned | 0.05 s |

The four rows are, in order:
1. today's `select {}`;
2. `<-r.Context().Done()` alone;
3. reading the body, then `<-r.Context().Done()`;
4. `select` on `r.Context().Done()` or a release channel closed before
   `srv.Close()`, which is the fix.

### A.2 Mutants of the fixed test

* `cb-handler-select-forever: exit=1 KILLED :: timed out (hang)`
* `cb-no-release: exit=1 KILLED :: timed out (hang)`
* `cb-counts-cancelled: exit=1 KILLED :: backplane_test.go:290: circuit breaker tripped on context cancellation — should be ignored`

### A.3 Stress

`go test -race -cpu 1,2,4 -count=1000 -timeout 600s -run
'^TestCircuitBreaker_IgnoresContextCancellation$' .`, while
`go test -count=3 ./llmprovider ./wizard` loaded the machine: 3,000 runs.

```text
ok  	github.com/maccavelli/mcplib	159.489s
```

## Appendix B — Diff

```diff
diff --git a/backplane_test.go b/backplane_test.go
--- a/backplane_test.go
+++ b/backplane_test.go
@@ -249,11 +249,20 @@
 }
 
 func TestCircuitBreaker_IgnoresContextCancellation(t *testing.T) {
-	// Server that always hangs (never responds).
-	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
-		select {} // block forever
-	}))
-	defer srv.Close()
+	// Server that holds every request until the client gives up or the test
+	// ends. It must return: srv.Close waits for running handlers, and with the
+	// body unread the server does not cancel r.Context() when the client
+	// leaves, so release is closed (deferred, before srv.Close) as well (MADR
+	// 0012 revision 4).
+	release := make(chan struct{})
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		select {
+		case <-r.Context().Done():
+		case <-release:
+		}
+	}))
+	defer srv.Close()
+	defer close(release)
 
 	c := &BackplaneClient{
 		addr:       srv.Listener.Addr().String(),
@@ -264,13 +273,16 @@
 	}
 	c.available.Store(true)
 
-	// Cancel context immediately — error is context cancellation, not network.
+	// An already-cancelled context fails before anything is sent.
 	for range 5 {
-		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
-		time.Sleep(2 * time.Millisecond) // ensure context expires
+		ctx, cancel := context.WithCancel(context.Background())
+		cancel()
 		_, _ = c.Generate(ctx, "hello")
-		cancel()
-	}
+	}
+	// A deadline that fires while the server holds the request.
+	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
+	_, _ = c.Generate(ctx, "hello")
+	cancel()
 
 	// Circuit breaker should NOT have tripped — context cancellation
 	// errors are not counted toward consecutive failures.
```
