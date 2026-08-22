// Package protocol defines the DBX wire protocol types and command registry.
package protocol

import "strings"

// DataType represents a Redis-compatible data type tag.
type DataType string

const (
	TypeString   DataType = "string"
	TypeHash     DataType = "hash"
	TypeList     DataType = "list"
	TypeSet      DataType = "set"
	TypeZSet     DataType = "zset"
	TypeStream   DataType = "stream"
	TypeDocument DataType = "document"
	TypeBitmap   DataType = "bitmap"
	TypeGeo      DataType = "geo"
	TypeVector   DataType = "vector"
	TypeNone     DataType = "none"
)

// Command represents a parsed client command.
type Command struct {
	Name string
	Args [][]byte
	// ClientID is set by the connection layer.
	ClientID uint64
}

// Arg returns argument at index i as a string (empty if out of bounds).
func (c *Command) Arg(i int) string {
	if i < len(c.Args) {
		return string(c.Args[i])
	}
	return ""
}

// ArgBytes returns argument at index i as bytes.
func (c *Command) ArgBytes(i int) []byte {
	if i < len(c.Args) {
		return c.Args[i]
	}
	return nil
}

// NumArgs returns the number of arguments (not counting the command name).
func (c *Command) NumArgs() int { return len(c.Args) }

// Normalized returns the uppercased command name.
func (c *Command) Normalized() string { return strings.ToUpper(c.Name) }

// CommandInfo describes a command's metadata.
type CommandInfo struct {
	Name      string
	Arity     int  // -N means at least N args
	ReadOnly  bool
	Admin     bool
	KeyIndex  int  // which arg is the key (0 = no key)
}

// Registry maps command names to their metadata.
var Registry = map[string]CommandInfo{
	// String commands
	"GET":      {Name: "GET", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"SET":      {Name: "SET", Arity: -3, KeyIndex: 1},
	"DEL":      {Name: "DEL", Arity: -2, KeyIndex: 1},
	"EXISTS":   {Name: "EXISTS", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"EXPIRE":   {Name: "EXPIRE", Arity: 3, KeyIndex: 1},
	"TTL":      {Name: "TTL", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"PERSIST":  {Name: "PERSIST", Arity: 2, KeyIndex: 1},
	"TYPE":     {Name: "TYPE", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"RENAME":   {Name: "RENAME", Arity: 3, KeyIndex: 1},
	"KEYS":     {Name: "KEYS", Arity: 2, ReadOnly: true, KeyIndex: 0},
	"SCAN":     {Name: "SCAN", Arity: -2, ReadOnly: true, KeyIndex: 0},
	"INCR":     {Name: "INCR", Arity: 2, KeyIndex: 1},
	"INCRBY":   {Name: "INCRBY", Arity: 3, KeyIndex: 1},
	"DECR":     {Name: "DECR", Arity: 2, KeyIndex: 1},
	"DECRBY":   {Name: "DECRBY", Arity: 3, KeyIndex: 1},
	"APPEND":   {Name: "APPEND", Arity: 3, KeyIndex: 1},
	"STRLEN":   {Name: "STRLEN", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"GETRANGE": {Name: "GETRANGE", Arity: 4, ReadOnly: true, KeyIndex: 1},
	"SETRANGE": {Name: "SETRANGE", Arity: 4, KeyIndex: 1},
	"MGET":     {Name: "MGET", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"MSET":     {Name: "MSET", Arity: -3, KeyIndex: 1},
	"SETNX":    {Name: "SETNX", Arity: 3, KeyIndex: 1},
	"GETSET":   {Name: "GETSET", Arity: 3, KeyIndex: 1},
	// Hash commands
	"HGET":    {Name: "HGET", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"HSET":    {Name: "HSET", Arity: -4, KeyIndex: 1},
	"HMGET":   {Name: "HMGET", Arity: -3, ReadOnly: true, KeyIndex: 1},
	"HMSET":   {Name: "HMSET", Arity: -4, KeyIndex: 1},
	"HDEL":    {Name: "HDEL", Arity: -3, KeyIndex: 1},
	"HGETALL": {Name: "HGETALL", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"HKEYS":   {Name: "HKEYS", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"HVALS":   {Name: "HVALS", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"HLEN":    {Name: "HLEN", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"HEXISTS": {Name: "HEXISTS", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"HINCRBY": {Name: "HINCRBY", Arity: 4, KeyIndex: 1},
	// List commands
	"LPUSH":   {Name: "LPUSH", Arity: -3, KeyIndex: 1},
	"RPUSH":   {Name: "RPUSH", Arity: -3, KeyIndex: 1},
	"LPOP":    {Name: "LPOP", Arity: -2, KeyIndex: 1},
	"RPOP":    {Name: "RPOP", Arity: -2, KeyIndex: 1},
	"LRANGE":  {Name: "LRANGE", Arity: 4, ReadOnly: true, KeyIndex: 1},
	"LLEN":    {Name: "LLEN", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"LINDEX":  {Name: "LINDEX", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"LSET":    {Name: "LSET", Arity: 4, KeyIndex: 1},
	"LREM":    {Name: "LREM", Arity: 4, KeyIndex: 1},
	"LTRIM":   {Name: "LTRIM", Arity: 4, KeyIndex: 1},
	// Set commands
	"SADD":        {Name: "SADD", Arity: -3, KeyIndex: 1},
	"SREM":        {Name: "SREM", Arity: -3, KeyIndex: 1},
	"SMEMBERS":    {Name: "SMEMBERS", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"SISMEMBER":   {Name: "SISMEMBER", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"SCARD":       {Name: "SCARD", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"SINTER":      {Name: "SINTER", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"SUNION":      {Name: "SUNION", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"SDIFF":       {Name: "SDIFF", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"SRANDMEMBER": {Name: "SRANDMEMBER", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"SPOP":        {Name: "SPOP", Arity: -2, KeyIndex: 1},
	// Sorted set commands
	"ZADD":            {Name: "ZADD", Arity: -4, KeyIndex: 1},
	"ZREM":            {Name: "ZREM", Arity: -3, KeyIndex: 1},
	"ZSCORE":          {Name: "ZSCORE", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"ZRANK":           {Name: "ZRANK", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"ZREVRANK":        {Name: "ZREVRANK", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"ZRANGE":          {Name: "ZRANGE", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"ZREVRANGE":       {Name: "ZREVRANGE", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"ZRANGEBYSCORE":   {Name: "ZRANGEBYSCORE", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"ZCARD":           {Name: "ZCARD", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"ZINCRBY":         {Name: "ZINCRBY", Arity: 4, KeyIndex: 1},
	"ZCOUNT":          {Name: "ZCOUNT", Arity: 4, ReadOnly: true, KeyIndex: 1},
	// Stream commands
	"XADD":    {Name: "XADD", Arity: -5, KeyIndex: 1},
	"XREAD":   {Name: "XREAD", Arity: -4, ReadOnly: true, KeyIndex: 0},
	"XRANGE":  {Name: "XRANGE", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"XREVRANGE": {Name: "XREVRANGE", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"XLEN":    {Name: "XLEN", Arity: 2, ReadOnly: true, KeyIndex: 1},
	"XGROUP":  {Name: "XGROUP", Arity: -2, KeyIndex: 2},
	"XREADGROUP": {Name: "XREADGROUP", Arity: -7, KeyIndex: 0},
	"XACK":    {Name: "XACK", Arity: -4, KeyIndex: 1},
	// Bitmap commands
	"SETBIT":   {Name: "SETBIT", Arity: 4, KeyIndex: 1},
	"GETBIT":   {Name: "GETBIT", Arity: 3, ReadOnly: true, KeyIndex: 1},
	"BITCOUNT": {Name: "BITCOUNT", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"BITPOS":   {Name: "BITPOS", Arity: -3, ReadOnly: true, KeyIndex: 1},
	// Geo commands
	"GEOADD":      {Name: "GEOADD", Arity: -5, KeyIndex: 1},
	"GEODIST":     {Name: "GEODIST", Arity: -4, ReadOnly: true, KeyIndex: 1},
	"GEORADIUS":   {Name: "GEORADIUS", Arity: -6, ReadOnly: true, KeyIndex: 1},
	"GEOPOS":      {Name: "GEOPOS", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"GEOSEARCH":   {Name: "GEOSEARCH", Arity: -7, ReadOnly: true, KeyIndex: 1},
	// Vector commands
	"VADD":        {Name: "VADD", Arity: -4, KeyIndex: 1},
	"VADD_BATCH":  {Name: "VADD_BATCH", Arity: -4, KeyIndex: 1},
	"VSEARCH":     {Name: "VSEARCH", Arity: -4, ReadOnly: true, KeyIndex: 1},
	// Pub/Sub commands
	"PUBLISH":     {Name: "PUBLISH", Arity: 3, KeyIndex: 0},
	"SUBSCRIBE":   {Name: "SUBSCRIBE", Arity: -2, KeyIndex: 0},
	"UNSUBSCRIBE": {Name: "UNSUBSCRIBE", Arity: -1, KeyIndex: 0},
	"PSUBSCRIBE":  {Name: "PSUBSCRIBE", Arity: -2, KeyIndex: 0},
	// Transaction commands
	"MULTI":   {Name: "MULTI", Arity: 1},
	"EXEC":    {Name: "EXEC", Arity: 1},
	"DISCARD": {Name: "DISCARD", Arity: 1},
	"WATCH":   {Name: "WATCH", Arity: -2, ReadOnly: true, KeyIndex: 1},
	"UNWATCH": {Name: "UNWATCH", Arity: 1},
	// Admin/Server commands
	"PING":    {Name: "PING", Arity: -1, ReadOnly: true},
	"ECHO":    {Name: "ECHO", Arity: 2, ReadOnly: true},
	"QUIT":    {Name: "QUIT", Arity: 1},
	"AUTH":    {Name: "AUTH", Arity: -2},
	"SELECT":  {Name: "SELECT", Arity: 2},
	"DBSIZE":  {Name: "DBSIZE", Arity: 1, ReadOnly: true},
	"FLUSHDB": {Name: "FLUSHDB", Arity: -1, Admin: true},
	"FLUSHALL": {Name: "FLUSHALL", Arity: -1, Admin: true},
	"INFO":    {Name: "INFO", Arity: -1, ReadOnly: true},
	"HELLO":   {Name: "HELLO", Arity: -1, ReadOnly: true},
	"CONFIG":  {Name: "CONFIG", Arity: -2, Admin: true},
	"DEBUG":   {Name: "DEBUG", Arity: -2, Admin: true},
	"SAVE":    {Name: "SAVE", Arity: 1, Admin: true},
	"BGSAVE":  {Name: "BGSAVE", Arity: -1, Admin: true},
	"CLUSTER": {Name: "CLUSTER", Arity: -2, Admin: true},
	"ACL":     {Name: "ACL", Arity: -2, Admin: true},
	"COMMAND": {Name: "COMMAND", Arity: -1, ReadOnly: true},
}

// Lookup retrieves command info by name (case-insensitive).
func Lookup(name string) (CommandInfo, bool) {
	info, ok := Registry[strings.ToUpper(name)]
	return info, ok
}
