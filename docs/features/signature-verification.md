## Signature Verification

By default signature verification is set to false (i.e the flag  --secure-policy needs to be used in the cli to enable signature verification)

The default setting for the policy.json file is usually found here /etc/containers/policy.json and typically should have these settings:

```bash

{
  "default": [
      {
          "type": "insecureAcceptAnything"
      }
  ],
  "transports":
    {
      "docker-daemon":
          {
              "": [{"type":"insecureAcceptAnything"}]
          },
      "docker":
        {
          "registry.redhat.io/redhat/certified-operator-index": [
            {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-isv"
            }
          ],
          "registry.redhat.io/redhat/community-operator-index": [
            {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-isv"
            }
          ],
          "registry.redhat.io/redhat/redhat-marketplace-index": [
            {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-isv"
            }
          ],
          "registry.redhat.io": [
            {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release"
            }
          ],
	        "registry.access.redhat.com": [
            {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release"
            }
          ],
          "quay.io/openshift-release-dev/openshift-release-dev" :[
 	          {
              "type": "signedBy",
              "keyType": "GPGKeys",
              "keyPath": "/etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release"
            }
	        ]
        }
    }
}

```

The default sigstore setting can be found here /etc/containers/registries.d/

Typically there should be entries for all the registries found in the policy.json file as an example for registry.redhat.io

A file for each entry found in the policy.json

Example file -> registry.redhat.io.yaml

```bash
docker:
     registry.redhat.io:
         sigstore: https://registry.redhat.io/containers/sigstore

```


Example file -> registry.access.redhat.com.yaml
 
```bash
docker:
     registry.access.redhat.com:
        sigstore: https://access.redhat.com/webassets/docker/content/sigstore

```

Example file -> quay.io.yaml (used for the quay.io/openshift-release-dev/openshift-release-dev entry)

```bash
docker:
     quay.io:
         sigstore: https://mirror.openshift.com/pub/openshift-v4/signatures

```