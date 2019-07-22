package main

import (
	"context"
	"os"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"strconv"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client

type Control struct {
	// (not yet used)
	ID        primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Detector string  `json:"detector,omitempty" bson:"detector,omitempty"`
	Mode     string  `json:"mode" bson:"mode,omitempty"`
	StopAfter int    `json:"stop_after" bson:"stop_after,omitempty"`
	Active  string   `json:"active,omitempty" bson:"active,omitempty"`
	User     string  `json:"user" bson:"user,omitempty"`
	Comment string   `json:"comment" bson:"comment,omitempty"`
	//some stuff about linked or not		
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

func main() {
	fmt.Println("Starting the API")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	clientOptions := options.Client().ApplyURI(os.Getenv("DAQ_MONGO_URI"))
	client, _ = mongo.Connect(ctx, clientOptions)
	router := mux.NewRouter()
	// router.HandleFunc("/setcommand/{detector}", UpdateCommandEndpoint).Methods("POST")
	router.HandleFunc("/getcommand/{detector}", AuthCheck(GetCommandEndpoint)).Methods("GET")
	router.HandleFunc("/getstatus/{host}", AuthCheck(GetStatusEndpoint)).Methods("GET")
	http.ListenAndServe(":12345", router)
}

func AuthCheck(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {

		fmt.Println(request.URL.Path)

		// Get auth params from URL
		user := request.FormValue("api_user")
		key := request.FormValue("api_key")
		fmt.Println(user)
		fmt.Println(key)
		if key == "" || user == ""{
			response.WriteHeader(http.StatusInternalServerError)
			response.Write([]byte(`{"message": "Access denied"}`))
			return
		}

		// Check against API key in database for this user
		
		// If it passes, then do this
		handlerFunc(response, request)
	}
}


func GetStatusEndpoint(response http.ResponseWriter, request *http.Request) {
	/*
               Provides endpoint /status/{host}, where {host} is the hostname
               of the node you're querying. By default provides only the most 
               recent status doc from the host, but can also provide documents
               for the last 'n' seconds with argument time_seconds=n, i.e.
               /status/{host}?time_seconds=3600 for the last hour.
        */
	
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	host := params["host"]
	time_seconds, err := strconv.Atoi(request.FormValue("time_seconds"))
	if err != nil{
		time_seconds = 0
	}

	// Mongo connectivity and context
	collection := client.Database("daq").Collection("status")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	// Set search options and make query
	options := options.Find()
	options.SetSort(bson.D{{"_id", -1}})
	query := bson.M{"host": host}
	if time_seconds > 0 {
		hex_time := fmt.Sprintf("%X", time.Now().Unix() - int64(time_seconds))
		query_oid, _ := primitive.ObjectIDFromHex(hex_time + "0000000000000000")
		query = bson.M{"host": host, "_id": bson.M{"$gte": query_oid}}
	} else {
		options.SetLimit(1)
	}
	cursor, err := collection.Find(ctx, query, options)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}

	// Package as json and return
	var statii []Status
	for cursor.Next(ctx) {
		var status Status
		cursor.Decode(&status)
		if time_seconds == 0 {
			json.NewEncoder(response).Encode(status)
			return
		} else {
			statii = append(statii, status);
		}
	}
	if len(statii) > 0 {
		json.NewEncoder(response).Encode(statii)
		return
	}
	
	// If we hit this point then the cursor was empty
	response.WriteHeader(http.StatusInternalServerError)
	response.Write([]byte(`{"message": "query returned no documents"}`))
	return
}


func GetCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	detector := params["detector"]

	// Mongo connectivity and context
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	// golang has no findOne? wtf
	cursor, err := collection.Find(ctx, bson.M{"detector": detector})
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}
	// Package as json and return
	for cursor.Next(ctx) {
		var control_doc Control
		cursor.Decode(&control_doc)
		json.NewEncoder(response).Encode(control_doc)
		return
	}
	// If we hit this point then the cursor was empty
	response.WriteHeader(http.StatusInternalServerError)
	response.Write([]byte(`{"message": "query returned no documents"}`))
	return
	
}
