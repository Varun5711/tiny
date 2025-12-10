#!/bin/bash

docker exec -i cassandra cqlsh << 'EOF'
CREATE KEYSPACE IF NOT EXISTS urlshortener
WITH replication = {
  'class': 'SimpleStrategy',
  'replication_factor': 1
};

USE urlshortener;

CREATE TABLE IF NOT EXISTS recent_clicks (
  short_code text,
  clicked_at timestamp,
  click_id timeuuid,
  ip_address text,
  user_agent text,
  referer text,
  PRIMARY KEY (short_code, clicked_at, click_id)
) WITH CLUSTERING ORDER BY (clicked_at DESC);

EOF