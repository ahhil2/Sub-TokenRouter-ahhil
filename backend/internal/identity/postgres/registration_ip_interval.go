package postgres

import (
	"context"
	"fmt"
	"net/netip"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// registration_ip_interval.go reserves an enabled registration interval in the
// account-creation transaction. The last successful registration is shared by
// all instances through PostgreSQL, whose clock determines whether the interval
// has elapsed. A conditional upsert serializes concurrent requests for the same
// address even when that address has no existing row. IPv4-mapped IPv6 addresses
// use the same stored identity as their IPv4 representation.
//
// The reservation commits with the account and invitation changes. Any later
// failure rolls it back, allowing another attempt without consuming the
// interval. Disabled policy skips reservations; changing an enabled duration
// applies that duration to the saved registration time on the next attempt.

// claimRegistrationIP reserves the supplied address under current interval
// settings. Call it inside the transaction that creates an email account;
// non-email account flows leave this operation unused. A disabled interval
// needs no database reservation, while missing dependencies or failed reads
// reject enabled enforcement. The upsert retains the address lock until the
// caller commits or rolls back, so competing registrations cannot both claim
// the same positive interval across separate application instances.
func (s *AuthState) claimRegistrationIP(ctx context.Context, address netip.Addr) error {
	if s.Settings == nil {
		return fmt.Errorf("registration interval settings are unavailable")
	}
	hours, err := s.Settings.GetRegistrationIPInterval(ctx)
	if err != nil {
		return fmt.Errorf("read registration interval: %w", err)
	}
	if hours == 0 {
		return nil
	}
	tx := dbent.TxFromContext(ctx)
	if tx == nil || !address.IsValid() {
		return fmt.Errorf("registration interval requires a transaction and valid client IP")
	}
	// The timestamp and row lock belong to account creation. A rejected attempt
	// leaves the stored timestamp intact; later account failures roll back this claim.
	result, err := tx.Client().ExecContext(ctx, `
INSERT INTO registration_ip_intervals (ip_address, last_registered_at)
VALUES ($1::inet, clock_timestamp())
ON CONFLICT (ip_address) DO UPDATE
SET last_registered_at = clock_timestamp()
WHERE registration_ip_intervals.last_registered_at
    <= clock_timestamp() - $2::integer * INTERVAL '1 hour'`, address.Unmap().WithZone("").String(), hours)
	if err != nil {
		return fmt.Errorf("claim registration interval: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read registration interval claim result: %w", err)
	}
	if affected == 0 {
		return identity.ErrRegistrationIPInterval
	}
	return nil
}
