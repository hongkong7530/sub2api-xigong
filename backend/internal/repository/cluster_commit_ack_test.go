package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
)

// The commit wrapper models a transport losing the ACK after the server committed.
// It never repeats the transaction and exposes the uncertain result to the caller.
type lostCommitACK struct {
	driver.Tx
	lose bool
}
func (tx *lostCommitACK) Commit() error {
	if err:=tx.Tx.Commit();err!=nil{return err}
	if tx.lose{return errors.New("fixture: commit acknowledgement lost")}
	return nil
}

type ackConnector struct { driver.Connector }
func(c ackConnector)Connect(ctx context.Context)(driver.Conn,error){
	conn,err:=c.Connector.Connect(ctx);if err!=nil{return nil,err};return &ackConnection{Conn:conn},nil
}
type ackConnection struct{ driver.Conn }
func(c *ackConnection)Begin()(driver.Tx,error){tx,err:=c.Conn.Begin();if err!=nil{return nil,err};return &lostCommitACK{Tx:tx,lose:true},nil}
func(c *ackConnection)BeginTx(ctx context.Context,options driver.TxOptions)(driver.Tx,error){
	if conn,ok:=c.Conn.(driver.ConnBeginTx);ok{tx,err:=conn.BeginTx(ctx,options);if err!=nil{return nil,err};return &lostCommitACK{Tx:tx,lose:true},nil}
	return c.Begin()
}
func(c *ackConnection)ExecContext(ctx context.Context,query string,args []driver.NamedValue)(driver.Result,error){if conn,ok:=c.Conn.(driver.ExecerContext);ok{return conn.ExecContext(ctx,query,args)};return nil,driver.ErrSkip}
func(c *ackConnection)QueryContext(ctx context.Context,query string,args []driver.NamedValue)(driver.Rows,error){if conn,ok:=c.Conn.(driver.QueryerContext);ok{return conn.QueryContext(ctx,query,args)};return nil,driver.ErrSkip}
func(c *ackConnection)Ping(ctx context.Context)error{if conn,ok:=c.Conn.(driver.Pinger);ok{return conn.Ping(ctx)};return nil}

type commitFixture struct{committed bool}
func(f *commitFixture)Commit()error{f.committed=true;return nil}
func(f *commitFixture)Rollback()error{return nil}
func TestClusterLostCommitACKFixture(t *testing.T){
	inner:=&commitFixture{};tx:=lostCommitACK{Tx:inner,lose:true}
	if tx.Commit()==nil||!inner.committed{t.Fatal("fixture must commit and then lose the acknowledgement")}
}
