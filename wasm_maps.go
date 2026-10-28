package glisp

// Maps aren't compiled yet (roadmap: CHAMP in wasm): their builtins fail in
// compiled code, saying so.
var uncompiledMapOps = map[string]bool{
	"map": true, "map_get": true, "map_put": true, "map_remove": true,
	"map_has": true, "map_count": true, "map_keys": true, "map_to_list": true,
}
