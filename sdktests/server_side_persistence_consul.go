package sdktests

import (
	"fmt"
	"strings"

	consul "github.com/hashicorp/consul/api"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type ConsulPersistentStore struct {
	consul *consul.Client
}

func (c *ConsulPersistentStore) DSN() string {
	//nolint:godox  // I'm working on it
	// TODO: Fix this address lookup
	return consul.DefaultConfig().Address
}

func (c *ConsulPersistentStore) Type() servicedef.SDKConfigPersistentType {
	return servicedef.Consul
}

func (c *ConsulPersistentStore) Reset() error {
	_, err := c.consul.KV().DeleteTree("/", nil)
	return err
}

func (c *ConsulPersistentStore) Get(prefix, key string) (o.Maybe[string], error) {
	kv := c.consul.KV()

	pair, _, err := kv.Get(prefix+"/"+key, nil)
	if err != nil || pair == nil {
		return o.None[string](), err
	}

	return o.Some(string(pair.Value)), nil
}

func (c *ConsulPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	kv := c.consul.KV()
	pairs, _, err := kv.List(prefix+"/"+key, nil)

	if err != nil {
		return nil, fmt.Errorf("list failed for %s: %s", key, err)
	}

	results := make(map[string]string)
	for _, pair := range pairs {
		flagKey := strings.TrimPrefix(pair.Key, prefix+"/"+key+"/")
		results[flagKey] = string(pair.Value)
	}
	return results, nil
}

func (c *ConsulPersistentStore) WriteMap(prefix, key string, data map[string]string) error {
	kv := c.consul.KV()

	// Start by reading the existing keys; we will later delete any of these
	// that weren't in data.
	pairs, _, err := kv.List(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to get existing items prior to Init: %s", err)
	}
	oldKeys := make(map[string]struct{})
	for _, p := range pairs {
		oldKeys[p.Key] = struct{}{}
	}

	ops := make([]*consul.KVTxnOp, 0)

	for k, flag := range data {
		op := &consul.KVTxnOp{Verb: consul.KVSet, Key: prefix + "/" + key + "/" + k, Value: []byte(flag)}
		ops = append(ops, op)

		delete(oldKeys, prefix+"/"+key+"/"+k)
	}

	for k := range oldKeys {
		op := &consul.KVTxnOp{Verb: consul.KVDelete, Key: prefix + "/" + key + "/" + k}
		ops = append(ops, op)
	}

	// Submit all the queued operations, using as many transactions as needed. (We're not really using
	// transactions for atomicity, since we're not atomic anyway if there's more than one transaction,
	// but batching them reduces the number of calls to the server.)
	return batchOperations(kv, ops)
}

// batchOperations handles applying a series of operations to Consult in batches of up to 64 operations.
//
// consul is limited to 64 operations per transaction, so any more than that must be split into multiple
// transactions.
func batchOperations(kv *consul.KV, ops []*consul.KVTxnOp) error {
	for i := 0; i < len(ops); {
		j := i + 64
		if j > len(ops) {
			j = len(ops)
		}
		batch := ops[i:j]
		ok, resp, _, err := kv.Txn(batch, nil)
		if err != nil {
			// COVERAGE: can't simulate this condition in unit tests because we will only get this
			// far if the initial query in Init() already succeeded, and we don't have the ability
			// to make the Consul client fail *selectively* within a single test
			return err
		}
		if !ok { // COVERAGE: see above
			errs := make([]string, 0)
			for _, te := range resp.Errors { // COVERAGE: see above
				errs = append(errs, te.What)
			}
			//nolint:stylecheck // this error message is capitalized on purpose
			return fmt.Errorf("Consul transaction failed: %s", strings.Join(errs, ", ")) // COVERAGE: see above
		}
		i = j
	}
	return nil
}
