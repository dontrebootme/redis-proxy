package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis"
	"github.com/go-redis/redis/v8"
	"github.com/karlseguin/ccache"
	"github.com/stretchr/testify/assert"
)

var redisClient *redis.Client
var redisServer *miniredis.Miniredis
var ctx context.Context
var lruCache *ccache.Cache
var config Config

type redisData struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

func TestUnitProxy_RedisGet(t *testing.T) {
	flag.Parse()
	if !testing.Short() {
		t.Skip()
	}
	setup()
	defer teardown()
	type fields struct {
		Config  Config
		Client  *redis.Client
		Context context.Context
		Cache   *ccache.Cache
	}
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "testUnitializedRedis",
			fields:  fields{},
			args:    args{key: "test"},
			want:    "",
			wantErr: true,
		},
		{
			name: "testRedisGetValid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "5f441d9c3cc37c4dd5afab8d"},
			want:    "Some data stored in redis",
			wantErr: false,
		},
		{
			name: "testRedisGetInvalid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "badkey"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Proxy{
				Config:  tt.fields.Config,
				Client:  tt.fields.Client,
				Context: tt.fields.Context,
				Cache:   tt.fields.Cache,
			}
			got, err := p.RedisGet(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Proxy.RedisGet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Proxy.RedisGet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnitProxy_CacheGet(t *testing.T) {
	flag.Parse()
	if !testing.Short() {
		t.Skip()
	}
	setup()
	defer teardown()
	type fields struct {
		Config  Config
		Client  *redis.Client
		Context context.Context
		Cache   *ccache.Cache
	}
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{

		{
			name:    "testUnitializedConfig",
			fields:  fields{},
			args:    args{key: "test"},
			want:    "",
			wantErr: true,
		},
		{
			name: "testCacheMissValid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "5f441d9c3cc37c4dd5afab8d"},
			want:    "Some data stored in redis",
			wantErr: false,
		},
		{
			name: "testCacheHitValid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "5f441d9c3cc37c4dd5afab8d"},
			want:    "Some data stored in redis",
			wantErr: false,
		},
		{
			name: "testCacheHitInvalid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "badkey"},
			want:    "",
			wantErr: true,
		},
		{
			name: "testCacheHitInvalid",
			fields: fields{
				Config:  config,
				Client:  redisClient,
				Context: ctx,
				Cache:   lruCache,
			},
			args:    args{key: "badkey"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Proxy{
				Config:  tt.fields.Config,
				Client:  tt.fields.Client,
				Context: tt.fields.Context,
				Cache:   tt.fields.Cache,
			}
			got, err := p.CacheGet(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Proxy.CacheGet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Proxy.CacheGet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mockRedis() *miniredis.Miniredis {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	return s
}

func loadRedisData() {
	file, err := ioutil.ReadFile("testdata/redis-data.json")
	if err != nil {
		panic(err)
	}

	redisEntries := []redisData{}
	_ = json.Unmarshal([]byte(file), &redisEntries)
	for i := 0; i < len(redisEntries); i++ {
		// fmt.Printf("About to SET key: %v, value: %v", redisEntries[i].Key, redisEntries[i].Value)
		redisClient.Set(ctx, redisEntries[i].Key, redisEntries[i].Value, time.Minute)
	}
	log.Printf("Loaded %v records into Redis", len(redisEntries))
}

func setup() {
	ctx = context.Background()
	redisServer = mockRedis()
	config = Config{
		RedisAddr: redisServer.Addr(),
		CacheTime: 120,
		CacheSize: 5000,
		Port:      "8080",
	}
	redisClient = redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
	})
	lruCache = ccache.New(ccache.Configure().MaxSize(config.CacheSize))

	loadRedisData()
}

func teardown() {
	redisServer.Close()
}

func TestEndtoEnd_Proxy(t *testing.T) {
	flag.Parse()
	if testing.Short() {
		t.Skip()
	}
	log.Println("beginning end to end testing")
	response := httptest.NewRecorder()
	ctx = context.Background()

	config = Config{
		RedisAddr: "redis:6379",
		CacheTime: 5,
		CacheSize: 10,
		Port:      "8080",
	}
	redisClient = redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
	})
	p := Proxy{
		Config:  config,
		Client:  redisClient,
		Context: ctx,
		Cache:   ccache.New(ccache.Configure().MaxSize(config.CacheSize)),
	}

	log.Println("loading Redis Data")
	loadRedisData()
	assert := assert.New(t)

	log.Println("validate cache does not have item")
	assert.Nil(p.Cache.Get("5f441d9c2c9193528dc9d64b"))

	log.Println("next get value from redis and put it in the cache")
	request, _ := http.NewRequest(http.MethodGet, "/5f441d9c2c9193528dc9d64b", nil)
	p.ServeHTTP(response, request)
	assert.Equal("More data in redis", response.Body.String())

	log.Println("validate item in cache")
	assert.Equal("More data in redis", p.Cache.Get("5f441d9c2c9193528dc9d64b").Value())
	_, err := p.CacheGet("5f441d9c2c9193528dc9d64b")
	assert.NoError(err)

	log.Println("wait for cache item to expire")
	time.Sleep(time.Duration(p.Config.CacheTime) * time.Second)

	log.Println("check that cache object has expired")
	assert.True(p.Cache.Get("5f441d9c2c9193528dc9d64b").Expired())

	log.Println("expired cache item should come from redis again.")
	val, err := p.CacheGet("5f441d9c2c9193528dc9d64b")
	assert.Equal("More data in redis", val)

	log.Println("validate item in cache after expired and retrieved")
	val, err = p.CacheGet("5f441d9c2c9193528dc9d64b")
	assert.Equal("More data in redis", val)

	log.Printf("request more than the number of items in the cache, current limit: %v", p.Config.CacheSize)
	keys := redisClient.Keys(ctx, "*").Val()
	// Request CacheSize+1 keys from API
	for i := 0; i <= int(p.Config.CacheSize); i++ {
		request, _ := http.NewRequest(http.MethodGet, "/"+keys[i], nil)
		p.ServeHTTP(response, request)
	}
	// Validate that the LRU item came out of the cache
	log.Println("check that oldest cache object has been removed from cache")
	assert.Nil(p.Cache.Get(keys[0]))
}
