package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/karlseguin/ccache"
	"github.com/segmentio/conf"
)

// Config holds our CLI flags and helper information for the redis-proxy command
type Config struct {
	RedisAddr string `conf:"redis-addr" help:"Address of the backing Redis (host:port)"`
	CacheTime int64  `conf:"cache-time" help:"Cache expiry time (seconds)"`
	CacheSize int64  `conf:"cache-size" help:"Capacity (number of keys)"`
	Port      string `conf:"port" help:"TCP/IP port number the proxy listens on"`
}

// Proxy data needed for configuration, redis client and LRU cache reuse between methods and consistent context for connections.
type Proxy struct {
	Config  Config
	Client  *redis.Client
	Context context.Context
	Cache   *ccache.Cache
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimLeft(r.URL.Path, "/")
	if len(key) < 1 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// log.Printf("Handling request for key: %v", key)
	val, err := p.CacheGet(key)
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}
	io.WriteString(w, val)
}

// CacheGet validates if key in accessible from cache and has not yet expired.
func (p *Proxy) CacheGet(key string) (string, error) {
	if p.Cache == nil {
		return "", fmt.Errorf("proxy not initialized")
	}

	// First check our cache to see if we have the key
	item := p.Cache.Get(key)

	// If we didnt find the key, or if we found it and the key was expired, then lets get it from Redis and add it to our cache
	if item == nil || item.Expired() {
		// Redis
		val, err := p.RedisGet(key)
		if err != nil {
			return "", err
		}
		// LRU Cache
		p.Cache.Set(key, val, time.Duration(p.Config.CacheTime)*time.Second)
		return val, nil
	}
	log.Printf("[cache] key: %v, val: %v, ttl: %v", key, item.Value(), item.TTL())

	if v, ok := item.Value().(string); ok {
		return v, nil
	}
	return "", fmt.Errorf("cacheget: could not decode type of '%T' for string", item.Value())
}

// RedisGet retreives key value from Redis backend
func (p *Proxy) RedisGet(key string) (string, error) {
	if p.Client == nil {
		return "", fmt.Errorf("redis not initialized")
	}
	val, err := p.Client.Get(p.Context, key).Result()
	if err != nil {
		return "", err
	}
	log.Printf("[redis] key: %v, val: %v", key, val)
	return val, nil
}

func main() {

	// Defaults for configuration/arguments
	p := Proxy{
		Config: Config{
			RedisAddr: "redis:6379",
			CacheTime: 120,
			CacheSize: 5000,
			Port:      "8080",
		},
		Context: context.Background(),
	}
	conf.Load(&p.Config)

	p.Client = redis.NewClient(&redis.Options{
		Addr: p.Config.RedisAddr,
	})

	p.Cache = ccache.New(ccache.Configure().MaxSize(p.Config.CacheSize))

	log.Print("Starting Redis Proxy")

	listenPort := ":" + p.Config.Port
	if err := http.ListenAndServe(listenPort, &p); err != nil {
		log.Fatal("Failed to start Redis Proxy: ", err)
	}
}
