package gateway

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestSessionLogBindingFilesASingleMembership(t *testing.T) {
	owner, team, org := sessionLogBinding("", "", "", []iam.Membership{{
		TeamID: "team-1", OrganizationID: "org-1", Role: "member",
	}})
	if owner != iam.OwnerPersonal || team != "team-1" || org != "org-1" {
		t.Fatalf("binding: owner=%q team=%q org=%q", owner, team, org)
	}
}

func TestSessionLogBindingDoesNotGuessAmongTeams(t *testing.T) {
	owner, team, org := sessionLogBinding("", "", "", []iam.Membership{
		{TeamID: "team-1", OrganizationID: "org-1"},
		{TeamID: "team-2", OrganizationID: "org-1"},
	})
	if owner != iam.OwnerPersonal || team != "" || org != "" {
		t.Fatalf("binding: owner=%q team=%q org=%q", owner, team, org)
	}
}
