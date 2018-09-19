all: pubkeys

# The following private keys are never regenerated.
SERVER_PRIVKEYS=chrome-ct-hub.privkey.pem test-ct-hub.privkey.pem

# Corresponding passwords:
HUB_CHROME_CT_PWD=pan-galactic-gargle-blaster
HUB_TEST_CT_PWD=arcturan-mega-gin

# Server public keys are derived from the corresponding private keys.
SERVER_PUBKEYS=$(subst .privkey,.pubkey,$(SERVER_PRIVKEYS))

# Build public keys from private keys
pubkeys: $(SERVER_PUBKEYS)
chrome-ct-hub.pubkey.pem: chrome-ct-hub.privkey.pem
	openssl ec -in $< -pubout -out $@ -passin pass:$(HUB_CHROME_CT_PWD)
test-ct-hub.pubkey.pem: test-ct-hub.privkey.pem
	openssl ec -in $< -pubout -out $@ -passin pass:$(HUB_TEST_CT_PWD)

# The newkey target creates a fresh private key; should never be needed.
newkey: fresh.privkey.pem
fresh.privkey.pem:
	openssl ecparam -genkey -name prime256v1 -noout -out $@.unencrypted
	openssl ec -in $@.unencrypted -out $@ -des  # Prompts for password
	rm -f $@.unencrypted
