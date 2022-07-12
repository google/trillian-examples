# OmniWitness: Monolith

See the [parent directory](../) for an introduction to what the OmniWitness is, and for
common configuration steps such as key generation and creating github credentials.

This version uses a single `main` construct to invoke all of the sub-components needed to
implement the OmniWitness. 

## Running

The configuration files referenced in the parent OmniWitness docs are compiled into the
application during build. The remaining configuration is that specific to the operator,
which is provided via flags (though see #662; this will change).

```
go run github.com/google/trillian-examples/witness/golang/omniwitness/monolith@master --alsologtostderr --v=1 \
  --private_key PRIVATE+KEY+my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --public_key my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --gh_user my-github-user \
  --gh_email foo@example.com \
  --gh_token ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --db_file ~/witness.db
```

