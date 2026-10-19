# Disk measurement retention and confidence

Disk measurement validity and statistical confidence are independent. A valid but variable observation is usable data; it is not proof of a sustainable maximum. A failed or interrupted fio invocation is not a valid observation, even if it produced some output.

## Root cause

Previously, `evaluate` returned earlier successful repetitions on a later error without their summary; `explorer.survey` and `finish` discarded those partial trials. `runDisk` also returned before attaching an interrupted search's history to its result. When every confirmation exceeded the CV threshold, `finish` left `Best` empty. Finally, the default renderer required MAX results to be confirmed, hiding otherwise usable measurements.

## Selection policy

Selection uses only this execution's samples that passed successful fio execution, the existing byte/count/rate/overflow and completion checks, active-direction checks, async-engine/direct-I/O validation, and achieved-depth verification. `Sample.DiskValidated` records this boundary; it does not bypass validation. Invalid samples are never eligible as unconfirmed observations.

1. Keep the highest stability-confirmed candidate selected by the existing optimizer. This takes priority over every higher but unconfirmed measurement. A later search failure does not invalidate an earlier confirmed measurement; it still makes the search incomplete and preserves its error and unsuccessful exit status.
2. Without a confirmed candidate, prefer the highest observed median among usable trials with at least three valid repetitions. Ties prefer more repetitions, then lower CV, then the first retained trial. An exploratory trial does not become confirmed merely because its CV is low: the existing fresh-confirmation requirements still apply.
3. If no such repeated trial exists, select the highest individual valid observation from retained trials, including successful repetitions before an interruption. Ties keep the first retained observation. Output labels it `Observation: Single sample`; two repetitions are not silently turned into a stable aggregate. The original repetition, configuration, runtime and per-job latency remain attached.
4. Without any usable observation, show `N/A`, `Confidence: NO VALID MEASUREMENT` and the real failure/not-run reason. Missing data never becomes zero. Previous executions are never used to fill a missing result.

Unconfirmed selections are always `Confidence: UNCONFIRMED`, with a concise reliability warning and detailed selection rationale. IOPS, bandwidth and latency use the same configuration and selected sample set. The existing median calculator is unchanged. P99 ranges contain separate job/run endpoints; percentiles are never averaged or pooled.

## Independent states

| Situation | Measurements | Confidence/search/execution |
| --- | --- | --- |
| Fresh repeated confirmation passed | Confirmed winner | CONFIRMED; configured ceiling still warns when reached |
| Confirmations too variable | Usable observed median or individual observation | UNCONFIRMED; completed with limitations |
| Too few repetitions | Explicit individual observation | UNCONFIRMED; no claim of stability |
| Later configuration/repetition failed or timed out | Earlier validated observations remain | Search incomplete; real error and unsuccessful exit status remain |
| Another Disk workload already completed | Its selected result remains | Each workload has its own confidence |
| No valid sample or workload never started | N/A with actual reason | NO VALID MEASUREMENT |
| Cleanup failed | Earlier valid metrics remain | Failure and retained-file warning remain |

QUICK remains preliminary and runs its existing one-repetition policy. STANDARD and MAX retain their original durations, repetition counts, block sizes, bounded depth/job settings, confirmation ranking and stopping behavior. CPU, RAM and Network selection policies are unchanged.

## Storage, safety and fallback policy

Retained samples use compact parsed `disk.Result` values with per-job counters/errors, runtime, means, percentiles, engine/workload options and observed depth. Unused JSON+ histogram bins are dropped from the retained copy; engine results are not mutated. Large raw JSON strings are not retained by the automatic controller, and no report files are created. Existing opt-in failed-fio diagnostics are unchanged.

One private dataset is prepared and reused as before. Cancellation still stops/reaps the child before identity-checked cleanup, and cleanup failure still stops further modules. The first already-tested baseline can supply a retained observation, but **no additional fallback workload or automatic retry is scheduled**. If that baseline itself fails validity checks, N/A remains. This optional fallback path was deliberately omitted to avoid extra load, retries after integrity/safety errors, or exceeding the remaining time budget. A workload not reached after an earlier fatal error remains N/A rather than being run solely for presentation.

## Changed implementation

| File | Functions/fields |
| --- | --- |
| `internal/benchmark/auto/optimizer.go` | `evaluate`, `explorer.survey`, `explorer.finish`: retain successful repetitions and summarize partial trials; add repetition, validation, confirmation-phase and failure metadata. Partial-trial retention is enabled only for Disk. |
| `internal/benchmark/auto/run.go` | `Run`, `runDisk`, extracted `searchDisk`: attach interrupted histories and independent workload placeholders, preserve original errors/cleanup, and select a usable observation without changing the bounded search. |
| `internal/benchmark/auto/disk_selection.go` | `UsableDiskTrial`, `SelectDiskResult`, `selectDiskObservation`, `compactDiskObservation`: eligibility, deterministic selection and compact retained evidence. |
| `internal/benchmark/disk/validation.go` | `ValidateObservation`: reuses existing `validateMeasured` and `ExecutionEvidence` at the controller boundary; no existing check is relaxed. |
| `internal/presentation/automatic_disk.go` | `diskResultView`, `renderCompactAutomaticDisk`, `diskConfidence`, `diskConfidenceWarning`: always show both read workloads, compatible metrics, confidence and no-data reasons. |
| `internal/presentation/automatic.go` | `RenderAutomatic`: use the same selection for default/details; show selection rationale, incomplete search and original failure. |
| `internal/presentation/automatic_compact.go` | `renderAutomaticCompact`: retain unconfirmed Disk data and avoid a maximum claim for unconfirmed observations. |
| `internal/presentation/automatic_quality.go` | `visibleAutomaticPeak`, `automaticDisplayStatus`: distinguish Disk availability from confidence while keeping failure status. |
| `internal/presentation/automatic_warnings.go` | `automaticWarnings`: retain validity failures and explain incomplete/variable/insufficient confirmation. |
| `internal/presentation/automatic_details.go` | `renderAutomaticDiskDetails`, `renderAutomaticTrials`: original repetitions, completion counters, phase, incomplete trials and actual failure diagnostics. |

Regression tests are in `internal/benchmark/auto/disk_retention_test.go` and `internal/presentation/automatic_disk_test.go`; existing automatic, formatting and unified-view fixtures were updated for the explicit validity/confidence contract. They use mocks and synthetic observations, without heavy workloads. Coverage includes confirmed/unconfirmed workloads, variability, insufficient repetitions, actual deadline expiry, QD ceilings, later failures, invalid/all-invalid output, single-sample consistency, no extra retries or cross-run reuse, cleanup and compact evidence.

Run `go test ./...` and `go vet ./...`. Real fio integration remains opt-in and requires an explicitly designated disposable test directory.
