package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/internal/adapters/repository"
	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/domain"
	"github.com/fineract/cacti-bridge/internal/service"
	"github.com/fineract/cacti-bridge/pkg/metrics"
	"github.com/fineract/cacti-bridge/test/mocks"
)

func newCoord(src, dst *mocks.MockLedger) (*service.Coordinator, *repository.MemoryRepository) {
	repo := repository.NewMemory()
	cfg := config.Default().Settlement // BurnMaxAttempts=4
	return service.NewCoordinator(src, dst, repo, cfg, metrics.New(), zap.NewNop()), repo
}

func req(ref string) domain.SettleRequest {
	return domain.SettleRequest{
		ReferenceID: ref, Amount: "100.00", Asset: "eAED",
		SourceLedger: "corda-uae", DestLedger: "besu-eu",
		Sender: "acct-a", Recipient: "acct-b",
	}
}

func TestSettle_HappyPath_LockReleaseBurn(t *testing.T) {
	src := &mocks.MockLedger{NameVal: "corda-uae"}
	dst := &mocks.MockLedger{NameVal: "besu-eu"}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R1"))
	require.NoError(t, err)
	assert.Equal(t, domain.StatusBurned, tr.Status)
	assert.NotEmpty(t, tr.LockTxID)
	assert.NotEmpty(t, tr.ReleaseTxID)
	assert.NotEmpty(t, tr.BurnTxID)
	assert.Equal(t, 1, src.Locks)
	assert.Equal(t, 1, dst.Releases)
	assert.Equal(t, 1, src.Burns)
}

func TestSettle_LockFails_Failed(t *testing.T) {
	src := &mocks.MockLedger{LockErr: errors.New("source down")}
	dst := &mocks.MockLedger{}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R2"))
	require.NoError(t, err)
	assert.Equal(t, domain.StatusFailed, tr.Status)
	assert.Equal(t, 0, dst.Releases) // never touched the destination
	assert.Contains(t, tr.FailureReason, "lock failed")
}

func TestSettle_ReleaseFails_CompensatesAndUnlocks(t *testing.T) {
	src := &mocks.MockLedger{}
	dst := &mocks.MockLedger{ReleaseErr: errors.New("dest rejected")}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R3"))
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCompensated, tr.Status)
	assert.Equal(t, 1, src.Locks)
	assert.Equal(t, 1, src.Unlocks) // compensation ran
	assert.NotEmpty(t, tr.UnlockTxID)
}

func TestSettle_BurnRetries_ThenSucceeds(t *testing.T) {
	src := &mocks.MockLedger{BurnFailTimes: 2} // fail twice, succeed on 3rd
	dst := &mocks.MockLedger{}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R4"))
	require.NoError(t, err)
	assert.Equal(t, domain.StatusBurned, tr.Status)
	assert.Equal(t, 3, src.Burns)
	assert.Equal(t, 3, tr.BurnAttempts)
}

func TestSettle_BurnExhausted_StaysReleased(t *testing.T) {
	src := &mocks.MockLedger{BurnFailTimes: 99} // never succeeds
	dst := &mocks.MockLedger{}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R5"))
	require.NoError(t, err)
	// Destination already credited -> never rolled back; left recoverable.
	assert.Equal(t, domain.StatusReleased, tr.Status)
	assert.Equal(t, 4, tr.BurnAttempts) // BurnMaxAttempts
	assert.Contains(t, tr.FailureReason, "burn pending")
}

func TestSettle_Idempotent_SameReference(t *testing.T) {
	src := &mocks.MockLedger{}
	dst := &mocks.MockLedger{}
	coord, _ := newCoord(src, dst)

	t1, err := coord.Settle(context.Background(), req("R6"))
	require.NoError(t, err)
	t2, err := coord.Settle(context.Background(), req("R6"))
	require.NoError(t, err)
	assert.Equal(t, t1.ID, t2.ID)
	assert.Equal(t, 1, src.Locks) // second call did not re-run the saga
}

func TestRollback_FromReleased_Refused(t *testing.T) {
	src := &mocks.MockLedger{BurnFailTimes: 99}
	dst := &mocks.MockLedger{}
	coord, _ := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R7"))
	require.NoError(t, err)
	require.Equal(t, domain.StatusReleased, tr.Status)

	_, err = coord.Rollback(context.Background(), tr.ID)
	require.Error(t, err)
	assert.Equal(t, domain.CodeConflict, domain.AsDomainError(err).Code)
}

func TestRecover_ResumesReleasedToBurned(t *testing.T) {
	// First settle leaves the transfer RELEASED (burn exhausted).
	src := &mocks.MockLedger{BurnFailTimes: 99}
	dst := &mocks.MockLedger{}
	coord, repo := newCoord(src, dst)

	tr, err := coord.Settle(context.Background(), req("R8"))
	require.NoError(t, err)
	require.Equal(t, domain.StatusReleased, tr.Status)

	// Source recovers: burns now succeed. Rebuild a coordinator over the same
	// repo with a healthy source and run recovery.
	src2 := &mocks.MockLedger{}
	cfg := config.Default().Settlement
	coord2 := service.NewCoordinator(src2, dst, repo, cfg, metrics.New(), zap.NewNop())

	n, err := coord2.Recover(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := repo.Get(context.Background(), tr.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusBurned, got.Status)
	assert.Equal(t, 1, src2.Burns) // completed the burn on recovery
}

func TestSettle_ValidationError(t *testing.T) {
	coord, _ := newCoord(&mocks.MockLedger{}, &mocks.MockLedger{})
	_, err := coord.Settle(context.Background(), domain.SettleRequest{ReferenceID: ""})
	require.Error(t, err)
	assert.Equal(t, domain.CodeValidation, domain.AsDomainError(err).Code)
}
