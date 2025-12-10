package cassandra

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type CassandraClient struct {
	session *gocql.Session
}

type Config struct {
	Hosts       []string
	Keyspace    string
	Consistency string
}

func NewCassandraClient(cfg Config) (*CassandraClient, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Keyspace = cfg.Keyspace
	cluster.Consistency = parseConsistency(cfg.Consistency)
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.ProtoVersion = 4

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Cassandra: %w", err)
	}

	return &CassandraClient{
		session: session,
	}, nil
}

func (c *CassandraClient) Close() {
	if c.session != nil {
		c.session.Close()
	}
}

func (c *CassandraClient) GetSession() *gocql.Session {
	return c.session
}

func parseConsistency(level string) gocql.Consistency {
	switch level {
	case "ONE":
		return gocql.One
	case "QUORUM":
		return gocql.Quorum
	case "ALL":
		return gocql.All
	case "LOCAL_QUORUM":
		return gocql.LocalQuorum
	default:
		return gocql.One
	}
}
