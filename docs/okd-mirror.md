# Mirror OKD

To pass signatures verification when trying to mirror OKD images, you need to set the following env variables: 
- OCP_SIGNATURE_URL="https://storage.googleapis.com/openshift-ci-release/releases/signatures/openshift/release/"
- OCP_SIGNATURE_VERIFICATION_PK="/path/to/PK" (recovered from "https://raw.githubusercontent.com/openshift/cluster-update-keys/master/keys/verifier-public-key-openshift-ci-4")

```
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    graph: false 
    channels:
      - name: 4-stable
        minVersion: 4.18.0-okd-scos.8
        maxVersion: 4.18.0-okd-scos.8
        type: okd
```