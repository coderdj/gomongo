package main

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// Document types
type Control struct {
	// Control document for setting the desired state of the system. 
	ID        primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Detector string  `json:"detector,omitempty" bson:"detector,omitempty"`
	Mode     string  `json:"mode,omitempty" bson:"mode,omitempty"`
	StopAfter string `json:"stop_after,omitempty" bson:"stop_after,omitempty"`
	Active  string   `json:"active,omitempty" bson:"active,omitempty"`
	User     string  `json:"user,omitempty" bson:"user,omitempty"`
	Comment string   `json:"comment,omitempty" bson:"comment,omitempty"`
	// The following two options only appear for 'tpc' and allow simultaneous
	// operation. 
	LinkMV  string   `json:"link_mv,omitempty" bson:"link_mv,omitempty"`
	LinkNV  string   `json:"link_nv,omitempty" bson:"link_nv,omitempty"`
}
type User struct{
	// Just get API key parameters
	APIUser string `bson:"api_username,omitempty"`
	APIKey  string `bson:"api_key,omitempty"`
}
type Status struct {
	ID    primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Host  string `json:"host,omitempty" bson:"host,omitempty"`
	Type  string `json:"type,omitempty" bson:"type,omitempty"`
	Status int32 `json:"status" bson:"status,omitempty"`
	Rate float64 `json:"rate" bson:"rate,omitempty"`
	BufferLength float64 `json:"buffer_length" bson:"buffer_length,omitempty"`
	RunMode string `json:"run_mode,omitempty" bson:"run_mode,omitempty"`
	Active []string `json:"active,omitempty" bson:"active,omitempty"`	
}
type DetectorStatus struct{
	ID   primitive.ObjectID `json:"_id, omitempty" bson:"_id,omitempty"`
	Status int32 `json:"status" bson:"status"`
	Number int32 `json:"number" bson:"number"`
	Detector string `json:"detector,omitempty" bson:"detector,omitempty"`
	Rate float64 `json:"rate" bson:"rate"`
	Readers int32 `json:"readers" bson:"readers"`
	Time time.Time `json:"time,omitempty" bson:"time,omitempty"`
	Buffer float64 `json:"buff" bson:"buff,omitempty"`
	Mode string `json:"mode,omitempty" bson:"mode,omitempty"`
}

