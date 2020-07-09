// A proxy that sends things on to tritter and logs the requests.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os/user"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian-examples/tritter/tritbot/log"
	"github.com/google/trillian-examples/tritter/tritter"
	tc "github.com/google/trillian/client"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
)

var (
	tritterAddr    = flag.String("tritter_addr", "localhost:50051", "the address of the tritter server")
	connectTimeout = flag.Duration("connect_timeout", time.Second, "the timeout for connecting to the server")
	sendTimeout    = flag.Duration("send_timeout", 10*time.Second, "the timeout for logging & sending each message")

	checkProof = flag.Bool("check_proof", false, "whether to confirm the data is logged before sending to tritter")
	loggerAddr = flag.String("logger_addr", "localhost:50052", "the address of the logger server")
)

type tritBot struct {
	c       tritter.TritterClient
	timeout time.Duration

	log log.LoggerClient
	v   *tc.LogVerifier

	tCon, lCon *grpc.ClientConn
}

func newTrustingTritBot(ctx context.Context) *tritBot {
	// Set up a connection to the Tritter server.
	tCon, err := grpc.DialContext(ctx, *tritterAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		glog.Fatalf("did not connect to tritter on %v: %v", *tritterAddr, err)
	}

	// Set up a connection to the Logger server.
	lCon, err := grpc.DialContext(ctx, *loggerAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		glog.Fatalf("did not connect to logger on %v: %v", *loggerAddr, err)
	}

	return &tritBot{
		c:       tritter.NewTritterClient(tCon),
		timeout: *sendTimeout,
		log:     log.NewLoggerClient(lCon),
		tCon:    tCon,
		lCon:    lCon,
	}
}

func newVerifyingTritBot(ctx context.Context) *tritBot {
	t := newTrustingTritBot(ctx)

	v, err := log.TreeVerifier()
	if err != nil {
		glog.Fatalf("could not create tree verifier: %v", err)
	}
	t.v = v
	return t
}

func (t *tritBot) Send(ctx context.Context, msg log.InternalMessage) error {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// First write the message to the log.
	r, err := t.log.Log(ctx, &log.LogRequest{Message: &msg})
	if err != nil {
		return err
	}

	// Second: check the message is in the log.
	if *checkProof {
		if r.GetProof() == nil {
			return errors.New("no proof to verify")
		}
		if t.v == nil {
			return errors.New("tritbot not configured with verifier")
		}

		root, err := t.v.VerifyRoot(&tt.LogRootV1{}, r.GetProof().GetRoot(), [][]byte{{}})
		if err != nil {
			return fmt.Errorf("failed to verify log root: %v", err)
		}

		bs := []byte(proto.MarshalTextString(&msg))
		if err := t.v.VerifyInclusionByHash(root, t.v.BuildLeaf(bs).MerkleLeafHash, r.GetProof().GetProof()); err != nil {
			return fmt.Errorf("could not verify inclusion proof: %v", err)
		}
		glog.Infof("verified proof for message %v", msg)
	}

	// Then continue to send the message to the server.
	_, err = t.c.Send(ctx, &tritter.SendRequest{Message: msg.GetMessage()})
	return err
}

func (t *tritBot) Close() error {
	if err := t.tCon.Close(); err != nil {
		return err
	}
	return t.lCon.Close()
}

func main() {
	flag.Parse()

	// Read the message from the argument list.
	if len(flag.Args()) == 0 {
		glog.Fatal("Required arguments: messages to send")
	}

	user, err := user.Current()
	if err != nil {
		glog.Fatalf("could not determine user: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *connectTimeout)
	defer cancel()

	// t := newTrustingTritBot(ctx)
	t := newVerifyingTritBot(ctx) // Use this when check_proof is required.
	defer t.Close()

	for _, msg := range flag.Args() {
		m := log.InternalMessage{
			User:      user.Username,
			Message:   msg,
			Timestamp: ptypes.TimestampNow(),
		}
		if err := t.Send(context.Background(), m); err != nil {
			glog.Fatalf("could not send message: %v", err)
		}
	}
	glog.Infof("Successfully sent %d messages", len(flag.Args()))
}
