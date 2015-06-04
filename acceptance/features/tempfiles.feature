Feature: temporary files

	Scenario: storing a public certificate as a literal in secrets.yml
		Given a file named "secrets.yml" with:
			"""
			GITHUB_CERTIFICATE: !file |
			  -----BEGIN CERTIFICATE-----
			  MIIF4DCCBMigAwIBAgIQDACTENIG2+M3VTWAEY3chzANBgkqhkiG9w0BAQsFADB1
			  [...]
			  XX4C2NesiZcLYbc2n7B9O+63M2k=
			  -----END CERTIFICATE-----
			"""
		When I successfully run `cauldron -p ./provider sh -c 'cat $GITHUB_CERTIFICATE'`
		Then the output should contain:
			"""
			-----BEGIN CERTIFICATE-----
			MIIF4DCCBMigAwIBAgIQDACTENIG2+M3VTWAEY3chzANBgkqhkiG9w0BAQsFADB1
			[...]
			XX4C2NesiZcLYbc2n7B9O+63M2k=
			-----END CERTIFICATE-----
			"""


	Scenario: using a private key from the secret store
		Given a file named "secrets.yml" with:
			"""
			PRIVATE_KEY: !file:var app/production/private-key
			"""

		And a secret "app/production/private-key" with:
			"""
			-----BEGIN RSA PRIVATE KEY-----
			MGMCAQACEQDVXDbwF2wjP5YDqtiNgpjFAgMBAAECECO12Hgc43uOhav8FFKGPyEC
			CQDyFyg5bf6PzQIJAOGeeNvh6YTZAgkAn0Heo1EZ2q0CCAXJe84gCE5ZAgkAt00H
			LezhFX0=
			-----END RSA PRIVATE KEY-----
			"""
		When I successfully run `cauldron -p ./provider sh -c 'cat $PRIVATE_KEY'`
		Then the output should contain:
			"""
			-----BEGIN RSA PRIVATE KEY-----
			MGMCAQACEQDVXDbwF2wjP5YDqtiNgpjFAgMBAAECECO12Hgc43uOhav8FFKGPyEC
			CQDyFyg5bf6PzQIJAOGeeNvh6YTZAgkAn0Heo1EZ2q0CCAXJe84gCE5ZAgkAt00H
			LezhFX0=
			-----END RSA PRIVATE KEY-----
			"""
