package config

import (
	"reflect"
	"sort"
	"testing"
)

// helper to grab names off a slice for assertions
func names(chains []RawChainConfig) []string {
	out := make([]string, len(chains))
	for i, c := range chains {
		out[i] = c.Name
	}
	sort.Strings(out)
	return out
}

func chainBSC(opts ...func(*RawChainConfig)) RawChainConfig {
	c := RawChainConfig{Name: "bsc", Id: "56", Endpoint: "http://bsc.local"}
	for _, o := range opts {
		o(&c)
	}
	return c
}

func chainMAP() RawChainConfig {
	return RawChainConfig{Name: "map", Id: "22776", Endpoint: "http://map.local"}
}

func TestDiffChains_AddOnly(t *testing.T) {
	old := []RawChainConfig{chainMAP(), chainBSC()}
	new := []RawChainConfig{chainMAP(), chainBSC(), {Name: "eth", Id: "1", Endpoint: "u"}}

	d := DiffChains(old, new)

	if got := names(d.Adds); !reflect.DeepEqual(got, []string{"eth"}) {
		t.Errorf("Adds = %v, want [eth]", got)
	}
	if len(d.Removes)+len(d.Restarts)+len(d.Updates) != 0 {
		t.Errorf("expected only Adds, got Removes=%v Restarts=%v Updates=%v",
			d.Removes, names(d.Restarts), names(d.Updates))
	}
}

func TestDiffChains_RemoveOnly(t *testing.T) {
	old := []RawChainConfig{chainMAP(), chainBSC()}
	new := []RawChainConfig{chainMAP()}

	d := DiffChains(old, new)

	if !reflect.DeepEqual(d.Removes, []string{"bsc"}) {
		t.Errorf("Removes = %v, want [bsc]", d.Removes)
	}
}

func TestDiffChains_StructuralChangeRestarts(t *testing.T) {
	old := []RawChainConfig{chainMAP(), chainBSC()}
	new := []RawChainConfig{chainMAP(), chainBSC(func(c *RawChainConfig) {
		c.Endpoint = "http://bsc.NEW"
	})}

	d := DiffChains(old, new)

	if got := names(d.Restarts); !reflect.DeepEqual(got, []string{"bsc"}) {
		t.Errorf("Restarts = %v, want [bsc]", got)
	}
	if len(d.Updates) != 0 {
		t.Errorf("did not expect Updates when endpoint changes, got %v", names(d.Updates))
	}
}

func TestDiffChains_NetworkChangeRestarts(t *testing.T) {
	old := []RawChainConfig{chainMAP(), chainBSC(func(c *RawChainConfig) { c.Network = "mainnet" })}
	new := []RawChainConfig{chainMAP(), chainBSC(func(c *RawChainConfig) { c.Network = "testnet" })}

	d := DiffChains(old, new)

	if got := names(d.Restarts); !reflect.DeepEqual(got, []string{"bsc"}) {
		t.Errorf("Restarts = %v, want [bsc]", got)
	}
}

func TestDiffChains_DataOnlyChangeUpdates(t *testing.T) {
	old := []RawChainConfig{chainMAP(), chainBSC(func(c *RawChainConfig) {
		c.Users = []From{{Group: "g1", From: "0xa"}}
	})}
	new := []RawChainConfig{chainMAP(), chainBSC(func(c *RawChainConfig) {
		c.Users = []From{{Group: "g1", From: "0xb"}}
	})}

	d := DiffChains(old, new)

	if got := names(d.Updates); !reflect.DeepEqual(got, []string{"bsc"}) {
		t.Errorf("Updates = %v, want [bsc]", got)
	}
	if len(d.Restarts) != 0 {
		t.Errorf("did not expect Restarts for data-only change")
	}
}

func TestDiffChains_NoChangeProducesEmptyDiff(t *testing.T) {
	chains := []RawChainConfig{chainMAP(), chainBSC()}
	d := DiffChains(chains, chains)
	if len(d.Adds)+len(d.Removes)+len(d.Restarts)+len(d.Updates) != 0 {
		t.Errorf("expected empty diff, got %+v", d)
	}
}

func TestDiffChains_MixedAddRemoveRestartUpdate(t *testing.T) {
	old := []RawChainConfig{
		chainMAP(),
		chainBSC(),
		{Name: "tron", Id: "728126428", Endpoint: "http://tron.old"},     // restart candidate
		{Name: "old-chain", Id: "999", Endpoint: "http://x"},             // remove
		{Name: "eth", Id: "1", Endpoint: "u", Users: []From{{Group: "g"}}}, // update
	}
	new := []RawChainConfig{
		chainMAP(),
		chainBSC(),
		{Name: "tron", Id: "728126428", Endpoint: "http://tron.NEW"}, // restart
		{Name: "new-chain", Id: "100", Endpoint: "http://y"},         // add
		{Name: "eth", Id: "1", Endpoint: "u", Users: []From{{Group: "g2"}}}, // update
	}

	d := DiffChains(old, new)

	if got := names(d.Adds); !reflect.DeepEqual(got, []string{"new-chain"}) {
		t.Errorf("Adds = %v, want [new-chain]", got)
	}
	if got := d.Removes; !reflect.DeepEqual(got, []string{"old-chain"}) {
		t.Errorf("Removes = %v, want [old-chain]", got)
	}
	if got := names(d.Restarts); !reflect.DeepEqual(got, []string{"tron"}) {
		t.Errorf("Restarts = %v, want [tron]", got)
	}
	if got := names(d.Updates); !reflect.DeepEqual(got, []string{"eth"}) {
		t.Errorf("Updates = %v, want [eth]", got)
	}
}
