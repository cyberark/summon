import os
import sys

print("Checking credentials")

haveSecrets = True

secretKeys = [ "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "MONGODB_PASSWORD" ]

for key in secretKeys:
  if key not in os.environ:
    print("%s not available!" % key)
    haveSecrets = False

if not haveSecrets:
  sys.exit()

print("Deploying application")

"""
Insert deployment script steps here!
"""
