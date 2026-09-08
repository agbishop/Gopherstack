package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// materializeServiceDir checks out repoRoot's services/ecs tree exactly as
// it existed at git rev rev (git archive, not a working-tree copy) into a
// fresh temp dir, test files included -- resolveServiceModules needs them
// to find ecs's SDK import (see modresolve.go's own doc comment on why
// test files matter for module resolution).
func materializeServiceDir(t *testing.T, repoRoot, rev string) string {
	t.Helper()

	const svcRelPath = "services/ecs"

	dst := t.TempDir()

	archive := exec.CommandContext(context.Background(), "git", "archive", rev, "--", svcRelPath)
	archive.Dir = repoRoot

	pipe, err := archive.StdoutPipe()
	require.NoError(t, err)

	untar := exec.CommandContext(context.Background(), "tar", "-x", "-C", dst)
	untar.Stdin = pipe

	require.NoError(t, archive.Start())
	require.NoError(t, untar.Start())
	require.NoError(t, archive.Wait())
	require.NoError(t, untar.Wait())

	return filepath.Join(dst, svcRelPath)
}

// TestScanServiceDir_ECSValidationBar is this tool's validation bar: it
// must flag every one of the eleven error codes commit c7817795 fixed in
// services/ecs (invented codes matching no real SDK type at all -- see
// main.go's doc comment) at the commit immediately before that fix, and it
// must flag NONE of them at the fix commit itself.
//
// errors.go's ServiceDeploymentAlreadyStoppedException is deliberately
// excluded from elevenCodes: it was never part of the originally-scoped
// eleven, and it is NOT a real ecs SDK code either (ecs@v1.90.0 models
// ServiceDeploymentNotFoundException, never an "AlreadyStopped" variant) --
// a twelfth invented code the original hand sweep missed. c7817795's much
// broader sweep fixed this one too, as an incidental side effect. See
// TestScanServiceDir_ECSTwelfthCodeAlsoFixed below.
func TestScanServiceDir_ECSValidationBar(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootDir()
	require.NoError(t, err)

	cache, err := gomodcacheDir(repoRoot)
	require.NoError(t, err)

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	require.NoError(t, err)

	elevenCodes := []string{
		"TaskNotFoundException",
		"ClusterAlreadyExistsException",
		"CapacityProviderNotFoundException",
		"CapacityProviderAlreadyExistsException",
		"TaskDefinitionNotFoundException",
		"ServiceAlreadyExistsException",
		"ContainerInstanceNotFoundException",
		"ExpressGatewayServiceNotFoundException",
		"ExpressGatewayServiceAlreadyExistsException",
		"AccountSettingNotFoundException",
	}

	t.Run("pre-fix flags all eleven invented codes", func(t *testing.T) {
		t.Parallel()

		dir := materializeServiceDir(t, repoRoot, "c781779587c7d14829f1828417452a9f9ce5ba49^")

		findings, scanErr := scanServiceDir(dir, repoRoot, cache, goModVersions)
		require.NoError(t, scanErr)

		flagged := map[string]bool{}

		for _, f := range findings {
			if f.Confident {
				flagged[f.Code] = true
			}
		}

		for _, code := range elevenCodes {
			require.Truef(
				t,
				flagged[code],
				"expected pre-fix ecs to confidently flag %s, findings: %+v",
				code,
				findings,
			)
		}
	})

	t.Run("post-fix flags none of the eleven", func(t *testing.T) {
		t.Parallel()

		dir := materializeServiceDir(t, repoRoot, "c781779587c7d14829f1828417452a9f9ce5ba49")

		findings, scanErr := scanServiceDir(dir, repoRoot, cache, goModVersions)
		require.NoError(t, scanErr)

		for _, f := range findings {
			for _, code := range elevenCodes {
				require.NotEqualf(
					t,
					code,
					f.Code,
					"post-fix ecs must not flag %s, but got: %+v",
					code,
					f,
				)
			}
		}
	})

	t.Run("post-fix flags no generic protocol codes", func(t *testing.T) {
		t.Parallel()

		dir := materializeServiceDir(t, repoRoot, "c781779587c7d14829f1828417452a9f9ce5ba49")

		findings, scanErr := scanServiceDir(dir, repoRoot, cache, goModVersions)
		require.NoError(t, scanErr)

		for _, f := range findings {
			require.Falsef(
				t, genericProtocolCodes[f.Code],
				"generic protocol code %s should never reach classify as a finding: %+v", f.Code, f,
			)
		}
	})
}

// TestScanServiceDir_ECSTwelfthCodeAlsoFixed documents a real finding this
// tool made during calibration: services/ecs/errors.go's
// ServiceDeploymentAlreadyStoppedException named no real ecs@v1.90.0 SDK
// type (confirmed by hand against types/errors.go, which declares
// ServiceDeploymentNotFoundException, never an "AlreadyStopped" variant) --
// a twelfth invented code the original eleven-code hand sweep missed,
// outside this tool's originally-scoped validation bar (see
// TestScanServiceDir_ECSValidationBar).
//
// This test originally pinned that finding as still-confidently-flagged at
// the fix commit, specifically so it would regress loudly if a future
// ground-truth change ever silently swallowed the gap. That's exactly what
// happened, just not silently: c7817795's much broader 222-bug sweep fixed
// this code too as an incidental side effect (renamed to ConflictException,
// the code ecs@v1.90.0's actual deserializer switch models for this
// condition -- confirmed via git show), even though it was never part of
// that commit's originally-scoped eleven. This test firing was the guard
// rail working as designed, not a bug -- it's rewritten here to confirm the
// fix stuck (mirroring TestScanServiceDir_ECSValidationBar's "post-fix
// flags none of the eleven" pattern) rather than to keep asserting a gap
// that no longer exists.
func TestScanServiceDir_ECSTwelfthCodeAlsoFixed(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootDir()
	require.NoError(t, err)

	cache, err := gomodcacheDir(repoRoot)
	require.NoError(t, err)

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	require.NoError(t, err)

	dir := materializeServiceDir(t, repoRoot, "c781779587c7d14829f1828417452a9f9ce5ba49")

	findings, err := scanServiceDir(dir, repoRoot, cache, goModVersions)
	require.NoError(t, err)

	for _, f := range findings {
		require.Falsef(
			t,
			f.Code == "ServiceDeploymentAlreadyStoppedException" && f.Confident,
			"post-fix ecs must no longer confidently flag ServiceDeploymentAlreadyStoppedException"+
				" (renamed to ConflictException by c7817795), but got: %+v",
			f,
		)
	}
}

// TestClassify_GenericListNeverOverridesRealServiceCode is gopherstack-udkm's
// confirmation for this tool: unlike cmd/errtargetaudit's per-operation
// declared set, classify's gt.codes is already the service-wide union
// (built the same way as errtargetaudit's AllCodes -- see
// serviceGroundTruth's doc comment), and this tool only ever asks "is this
// code real ANYWHERE in this service's own SDK" -- never "for THIS
// operation" (that's errtargetaudit's job, gopherstack-o46l's class). So a
// code present in gt.codes is excluded because it's real, and whether it
// also happens to sit in genericProtocolCodes changes nothing: no source
// change was needed here. This locks that in.
func TestClassify_GenericListNeverOverridesRealServiceCode(t *testing.T) {
	t.Parallel()

	const code = "ValidationException"

	gt := &serviceGroundTruth{codes: map[string]bool{code: true}}
	cands := []candidate{{File: "f.go", Line: 1, Code: code, Mechanism: mechStdlibErr}}

	findings := classify(cands, gt)

	require.Empty(t, findings,
		"a code real in gt.codes must never be reported, generic-list membership notwithstanding")
}

// TestClassify_GenericListOnlyExcusesWhenAbsentFromServiceCodes is the
// module-conditional half: a genuinely generic code THIS service's own
// resolved module never declares anywhere (gt.codes lacks it) stays
// excused, while a fabricated code -- absent from both gt.codes and the
// generic list -- is still flagged.
func TestClassify_GenericListOnlyExcusesWhenAbsentFromServiceCodes(t *testing.T) {
	t.Parallel()

	gt := &serviceGroundTruth{codes: map[string]bool{}}
	cands := []candidate{
		{File: "f.go", Line: 1, Code: "Throttling", Mechanism: mechStdlibErr},
		{File: "f.go", Line: 2, Code: "TotallyFabricatedException", Mechanism: mechStdlibErr},
	}

	findings := classify(cands, gt)

	require.Len(t, findings, 1)
	require.Equal(t, "TotallyFabricatedException", findings[0].Code)
}

// TestScanServiceDir_SkipsNoGroundTruth confirms ec2 -- whose OWN pinned
// SDK module models zero error codes at all (see moduleCodes's doc
// comment) -- never produces a CONFIDENT finding, matching commit
// c7817795's own documented conclusion that ec2 needed no change because
// there was nothing to check against. It may still produce NEEDS-REVIEW
// findings: one *_test.go file imports outposts for an unrelated
// cross-service integration test, which makes resolvedModules 2 (ec2 +
// outposts) and demotes anything found there rather than silently
// checking ec2's own emissions against outposts's exception set (see
// serviceGroundTruth's doc comment) -- that demotion, not silence, is the
// behavior under test here.
func TestScanServiceDir_SkipsNoGroundTruth(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootDir()
	require.NoError(t, err)

	cache, err := gomodcacheDir(repoRoot)
	require.NoError(t, err)

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(repoRoot, "services", "ec2"))
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	findings, err := scanServiceDir(
		filepath.Join(repoRoot, "services", "ec2"),
		repoRoot,
		cache,
		goModVersions,
	)
	require.NoError(t, err)

	for _, f := range findings {
		require.Falsef(
			t,
			f.Confident,
			"ec2 has no ground truth of its own to check against; got confident finding: %+v",
			f,
		)
	}
}

// TestScanServiceDir_PersonalizeInternalServerException is the real
// false positive gopherstack-oshm's InternalServerException addition to
// genericProtocolCodes fixes: personalize's two resolved modules
// (personalize, personalizeruntime -- test-file imports make it 2, so any
// finding here demotes to needs-review, never confident) model
// InternalServerException nowhere in either's types/errors.go or
// deserializers.go (confirmed by grep against both pinned modules), yet
// handler.go emits it as a literal genuine unexpected-failure fallback
// twice. InternalServerException is real AWS nomenclature -- modeled
// per-op in 51 of the pinned SDK's other service modules, including mgn
// (cmd/errtargetaudit/genericcodes.go's citation for the same code) --
// personalize just isn't one of the 51, which is a "real code, wrong
// service" gap outside this tool's own class B scope, not evidence the
// code is fabricated.
func TestScanServiceDir_PersonalizeInternalServerExceptionReported(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootDir()
	require.NoError(t, err)

	cache, err := gomodcacheDir(repoRoot)
	require.NoError(t, err)

	goModVersions, err := loadGoModVersions(filepath.Join(repoRoot, "go.mod"))
	require.NoError(t, err)

	findings, err := scanServiceDir(
		filepath.Join(repoRoot, "services", "personalize"),
		repoRoot,
		cache,
		goModVersions,
	)
	require.NoError(t, err)

	var got int

	for _, f := range findings {
		if f.Code == "InternalServerException" {
			got++
		}
	}

	require.Positive(t, got,
		"personalize@v1.50.4 declares no server-fault type at all, so its "+
			"InternalServerException emissions must be reported, not suppressed "+
			"by genericProtocolCodes (gopherstack-oshm)")
}
