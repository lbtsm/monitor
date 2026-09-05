package config

// ApplyHotReloadable copies every hot-reloadable field from source onto
// target in place. Fields classified as immutable (Endpoint, KeystorePath,
// Type, Id, ChangeInterval, CheckHgtCount, MapChainID, gas params,
// StartBlock, Name) are left untouched so a structural change must instead
// go through the chain Restart path.
//
// Pointer fields (Tk, Genni, Tss) are repointed to source's pointers so
// callers holding the same OptConfig pointer (each per-chain monitor)
// transparently see the new values on their next Snapshot.
//
// Slice fields (From, Users, ContractToken, Energies) are reassigned to
// source's slice headers, which is safe because polling loops snapshot
// before iterating.
//
// Caller is responsible for holding the OptConfig's writer lock (e.g. via
// chain.Common.UpdateCfg).
func ApplyHotReloadable(target, source *OptConfig) {
	if source == nil {
		panic("config: ApplyHotReloadable called with nil source")
	}
	target.WaterLine = source.WaterLine
	target.LightNode = source.LightNode
	target.ApiUrl = source.ApiUrl
	target.From = source.From
	target.Users = source.Users
	target.ContractToken = source.ContractToken
	target.Energies = source.Energies
	target.Tss = source.Tss
	target.Tk = source.Tk
	target.Genni = source.Genni
}
