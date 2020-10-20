---
automation:
  - INTLY-7748
products:
  - name: rhoam
    environments:
      - osd-post-upgrade
      - osd-fresh-install
estimate: 15m
tags:
  - per-release
---

# B03b - Verify RHOAM Developer User Permissions are Correct

**Automated Test**: [user_rhmi_developer_permissions.go](https://github.com/integr8ly/integreatly-operator/blob/master/test/common/user_rhmi_developer_permissions.go)

## Steps

The following steps are still not automated in [user_rhmi_developer_permissions.go](https://github.com/integr8ly/integreatly-operator/blob/master/test/common/user_rhmi_developer_permissions.go). Once automated, the manual steps should be removed from this test case.

### No Access to RHMI Custom Resource

JIRA: [INTLY-7792](https://issues.redhat.com/browse/INTLY-7792)

1. Navigate to the console & log in as an RHMI developer user (e.g. as a test-userXX)
2. Go to **Home** > **Search**
3. Select **RHMI** from the custom resource dropdown
   > Verify that you are not be able to view any RHMI custom resources
