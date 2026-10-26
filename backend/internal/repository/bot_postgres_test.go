//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// This test runs only against a disposable database explicitly supplied by the test operator.
func TestBotPostgresQueriesAndMigrationACL(t *testing.T) {
	dsn := os.Getenv("SUBNEXUS_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable PostgreSQL DSN not supplied")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))
	require.True(t, strings.HasPrefix(name, "subnexus_test_"), "refuse a non-test database")
	defer db.Exec(`DROP OWNED BY bot_fixture_ingress,bot_fixture_worker; DROP ROLE bot_fixture_ingress; DROP ROLE bot_fixture_worker;`)
	_, e = db.Exec(`CREATE ROLE bot_fixture_ingress NOLOGIN; CREATE ROLE bot_fixture_worker NOLOGIN;
 CREATE TABLE cluster_nodes(cluster_id text,node_id text,roles jsonb);
 CREATE TABLE cluster_node_credentials(cluster_id text,node_id text,database_role text,status text);
 INSERT INTO cluster_nodes VALUES('test','ingress','["ingress","worker"]'),('test','worker','["worker"]');
 ALTER ROLE bot_fixture_ingress LOGIN; ALTER ROLE bot_fixture_worker LOGIN;
 INSERT INTO cluster_node_credentials VALUES('test','ingress','bot_fixture_ingress','active'),('test','worker','bot_fixture_worker','active');`)
	require.NoError(t, e)
	migration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "282_subnexus_bot.sql"))
	require.NoError(t, e)
	_, e = db.Exec(string(migration))
	require.NoError(t, e)
	var ingressRead, ingressInsert, ingressUpdate, ingressEdit, workerRead bool
	e = db.QueryRow(`SELECT has_table_privilege('bot_fixture_ingress','bot_installations','SELECT'),has_table_privilege('bot_fixture_ingress','bot_installations','INSERT'),has_column_privilege('bot_fixture_ingress','bot_installations','revoked_at','UPDATE'),has_column_privilege('bot_fixture_ingress','bot_installations','policy','UPDATE'),has_table_privilege('bot_fixture_worker','bot_installations','SELECT')`).Scan(&ingressRead, &ingressInsert, &ingressUpdate, &ingressEdit, &workerRead)
	require.NoError(t, e)
	require.True(t, ingressRead)
	require.True(t, ingressInsert)
	require.True(t, ingressUpdate)
	require.False(t, ingressEdit)
	require.False(t, workerRead)
	_, e = db.Exec(`CREATE TABLE groups(id bigint,platform text,deleted_at timestamptz,rate_multiplier numeric,model_pricing jsonb);
 CREATE TABLE accounts(id bigint,platform text,type text,status text,schedulable boolean,rate_limit_reset_at timestamptz,overload_until timestamptz,last_used_at timestamptz,deleted_at timestamptz,credentials jsonb);
 CREATE TABLE account_groups(account_id bigint,group_id bigint);
 CREATE TABLE channels(id bigint,status text);
 CREATE TABLE channel_groups(channel_id bigint,group_id bigint);
 CREATE TABLE site_profit_meta(id int,started_at timestamptz,timezone text);
 CREATE TABLE site_profit_entries(user_id bigint,group_id bigint,platform text,billing_type smallint,settled_at timestamptz,revenue_usd numeric,cost_usd numeric,profit_usd numeric);
 CREATE TABLE usage_logs(id bigint,user_id bigint,group_id bigint,account_id bigint,channel_id bigint,model text,requested_model text,upstream_model text,upstream_response_model text,upstream_model_mismatch boolean,created_at timestamptz,input_tokens int,output_tokens int,cache_read_tokens int,cache_creation_tokens int,actual_cost numeric,total_cost numeric,first_token_ms int,duration_ms int);
 INSERT INTO groups VALUES(7,'openai',NULL,0.3,'[{"models":["fixture-model"],"input_price":1,"credentials":"must-not-leak"}]'),(8,'openai',NULL,0.5,'[]');
 INSERT INTO accounts VALUES(10,'openai','apikey','active',true,NULL,NULL,now(),NULL,'{"api_key":"must-not-leak"}'),(11,'openai','apikey','active',true,NULL,NULL,now(),NULL,'{}');
 INSERT INTO account_groups VALUES(10,7),(11,8);
 INSERT INTO channels VALUES(20,'active'),(21,'active'); INSERT INTO channel_groups VALUES(20,7),(21,8);
 INSERT INTO site_profit_meta VALUES(1,now()-interval '1 hour','Asia/Shanghai');
 INSERT INTO site_profit_entries VALUES(9,7,'openai',0,now(),0.3,0.2,0.1),(10,8,'openai',0,now(),100,80,20),(10,7,'openai',0,now(),0.6,NULL,NULL);
 INSERT INTO usage_logs VALUES(1,9,7,10,20,'fixture-model','fixture-model','mapped','declared',true,now(),10,20,3,4,0.3,1,12,100),(2,10,8,11,21,'hidden','hidden','hidden','hidden',false,now(),900,900,0,0,100,100,1,1);`)
	require.NoError(t, e)
	r := NewBotRepository(db)
	v := &service.BotInstallation{BotPolicy: service.BotPolicy{GroupIDs: []int64{7}}}
	for _, route := range []string{"costs", "profit", "consumption", "logs", "model-audit", "accounts/status", "channels/status", "models/prices"} {
		t.Run(route, func(t *testing.T) {
			q, e := service.ParseBotQuery(route, "", time.Now(), v)
			require.NoError(t, e)
			raw, e := r.Query(context.Background(), route, v, q)
			require.NoError(t, e)
			require.True(t, json.Valid(raw))
			require.NotContains(t, string(raw), "must-not-leak")
			require.NotContains(t, string(raw), "hidden")
			require.NotContains(t, string(raw), `"group_id":8`)
			require.NotContains(t, string(raw), `"id":11`)
			require.NotContains(t, string(raw), `"id":21`)
			if route == "accounts/status" || route == "channels/status" {
				require.Contains(t, string(raw), `"status_kind":"configured_state_not_runtime_health"`)
			}
			if route == "costs" {
				require.Contains(t, string(raw), `"unknown_cost_requests":1`)
				require.Contains(t, string(raw), `"requests":2`)
			}
		})
	}
	// Unmapped and legacy blank model columns retain the effective upstream model.
	_, e = db.Exec(`UPDATE usage_logs SET requested_model='',upstream_model=NULL,upstream_response_model=NULL WHERE id=1`)
	require.NoError(t, e)
	for _, route := range []string{"consumption", "logs", "model-audit"} {
		q, err := service.ParseBotQuery(route, "requested_model=fixture-model&upstream_model=fixture-model", time.Now(), v)
		require.NoError(t, err)
		raw, err := r.Query(context.Background(), route, v, q)
		require.NoError(t, err)
		if route == "consumption" {
			var result struct { Requests int64 `json:"requests"` }
			require.NoError(t, json.Unmarshal(raw, &result))
			require.EqualValues(t, 1, result.Requests)
		} else {
			require.Contains(t, string(raw), `"upstream_model":"fixture-model"`)
			require.Contains(t, string(raw), `"requested_model":"fixture-model"`)
			require.Contains(t, string(raw), `"upstream_response_model":null`)
		}
	}
	v.UserIDs = []int64{9}
	q, e := service.ParseBotQuery("costs", "", time.Now(), v)
	require.NoError(t, e)
	raw, e := r.Query(context.Background(), "costs", v, q)
	require.NoError(t, e)
	require.Contains(t, string(raw), `"requests":1`)
	require.Contains(t, string(raw), `"unknown_cost_requests":0`)
}
