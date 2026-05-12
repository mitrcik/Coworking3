package models

import "time"

// Roles ----------------------------------------------------------------------

type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
)

// WorkspaceType --------------------------------------------------------------

type WorkspaceType string

const (
	WorkspaceDesk        WorkspaceType = "DESK"
	WorkspaceMeetingRoom WorkspaceType = "MEETING_ROOM"
	WorkspaceLounge      WorkspaceType = "LOUNGE"
)

// BookingStatus --------------------------------------------------------------

type BookingStatus string

const (
	StatusConfirmed         BookingStatus = "CONFIRMED"
	StatusCompleted         BookingStatus = "COMPLETED"
	StatusCancelledByUser   BookingStatus = "CANCELLED_BY_USER"
	StatusCancelledByAdmin  BookingStatus = "CANCELLED_BY_ADMIN"
)

// NotificationType -----------------------------------------------------------

type NotificationType string

const (
	NotificationBookingConfirmed NotificationType = "BOOKING_CONFIRMED"
	NotificationBookingCancelled NotificationType = "BOOKING_CANCELLED"
	NotificationReminder         NotificationType = "REMINDER"
)

// Entities -------------------------------------------------------------------

type User struct {
	ID                 string
	FullName           string
	Email              string
	PasswordHash       string
	Role               Role
	ActiveBookingCount int
	CreatedAt          time.Time
}

type Coworking struct {
	ID        string
	Name      string
	GridCols  int
	GridRows  int
	CreatedAt time.Time
}

type Workspace struct {
	ID          string
	CoworkingID string
	Name        string
	Type        WorkspaceType
	Zone        string
	IsAvailable bool
	PositionX   int
	PositionY   int
	CreatedAt   time.Time
}

type Booking struct {
	ID           string
	UserID       string
	WorkspaceID  string
	StartTime    time.Time
	EndTime      time.Time
	Status       BookingStatus
	CreatedAt    time.Time
	CancelledAt  *time.Time
}

type BookingSettings struct {
	ID                       string
	MaxActiveBookingsPerUser int
	UpdatedBy                *string
	UpdatedAt                time.Time
}

type Report struct {
	ID                  string
	GeneratedAt         time.Time
	DateRangeStart      time.Time
	DateRangeEnd        time.Time
	WorkspaceTypeFilter *WorkspaceType
	Data                []byte
	CreatedBy           *string
}

type Notification struct {
	ID         string
	UserID     string
	BookingID  *string
	Type       NotificationType
	Message    string
	SentAt     time.Time
	IsRead     bool
}
