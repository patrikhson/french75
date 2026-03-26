package notification

// Notification types sent to regular users.
const (
	TypeCheckinApproved      = "checkin_approved"
	TypeCheckinRejected      = "checkin_rejected"
	TypeDrinkRequestApproved = "drink_request_approved"
	TypeDrinkRequestRejected = "drink_request_rejected"
	TypeAccountApproved      = "account_approved"
	TypeNewFollower              = "new_follower"
	TypeCheckinReaction          = "checkin_reaction"
	TypeFollowRequestReceived    = "follow_request_received"
	TypeFollowRequestApproved    = "follow_request_approved"
	TypeFollowRequestRejected    = "follow_request_rejected"
)

// Notification types sent to admin users.
const (
	TypeAdminNewRegistration = "admin_new_registration"
	TypeAdminNewCheckin      = "admin_new_checkin"
	TypeAdminNewDrinkRequest = "admin_new_drink_request"
	TypeAdminSpamFlag        = "admin_spam_flag"
)

// AllTypes lists every notification type in a stable order for the preferences UI.
var AllTypes = []string{
	TypeCheckinApproved,
	TypeCheckinRejected,
	TypeDrinkRequestApproved,
	TypeDrinkRequestRejected,
	TypeAccountApproved,
	TypeNewFollower,
	TypeCheckinReaction,
	TypeFollowRequestReceived,
	TypeFollowRequestApproved,
	TypeFollowRequestRejected,
	TypeAdminNewRegistration,
	TypeAdminNewCheckin,
	TypeAdminNewDrinkRequest,
	TypeAdminSpamFlag,
}

// TypeLabel returns a human-readable label for a notification type.
func TypeLabel(t string) string {
	switch t {
	case TypeCheckinApproved:
		return "Check-in approved"
	case TypeCheckinRejected:
		return "Check-in rejected"
	case TypeDrinkRequestApproved:
		return "Drink request approved"
	case TypeDrinkRequestRejected:
		return "Drink request rejected"
	case TypeAccountApproved:
		return "Account approved"
	case TypeNewFollower:
		return "New follower"
	case TypeCheckinReaction:
		return "Reaction on your check-in"
	case TypeFollowRequestReceived:
		return "Follow request received"
	case TypeFollowRequestApproved:
		return "Follow request approved"
	case TypeFollowRequestRejected:
		return "Follow request not approved"
	case TypeAdminNewRegistration:
		return "New registration (admin)"
	case TypeAdminNewCheckin:
		return "New check-in pending (admin)"
	case TypeAdminNewDrinkRequest:
		return "New drink request (admin)"
	case TypeAdminSpamFlag:
		return "Check-in flagged as spam (admin)"
	}
	return t
}

// IsAdminType returns true for notification types that are admin-only.
func IsAdminType(t string) bool {
	switch t {
	case TypeAdminNewRegistration, TypeAdminNewCheckin, TypeAdminNewDrinkRequest, TypeAdminSpamFlag:
		return true
	}
	return false
}
