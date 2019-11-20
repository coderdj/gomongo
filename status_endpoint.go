package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"errors"
	"github.com/gorilla/mux"	
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

func GetDetectorStatusEndpoint(response http.ResponseWriter, request *http.Request) {

	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	detector := params["detector"]
	status, err := GetDetectorStatus(detector, 30);
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "No status update for 30 seconds"}`))
		return
	}
	json.NewEncoder(response).Encode(status)
	return
		
}

func GetErrorsEndpoint(response http.ResponseWriter, request *http.Request){

	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	min_level, err := strconv.Atoi(params["level"])
	if err != nil {
		min_level = 2
	}
	errors, err := GetErrors(min_level)
	if err != nil{
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "No response when querying for errors"}`))
		return
	}
	json.NewEncoder(response).Encode(errors)
	return
}

func GetErrors(min_level int) ([]LogEntry, error){
	// 0-debug, 1-message, 2-warning, 3-error, 4-fatal, 5-user//
	collection := client.Database("daq").Collection("log")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	options := options.Find()
	options.SetSort(bson.D{{"_id", -1}})
	options.SetLimit(1)
	cursor, err := collection.Find(ctx, bson.M{"priority": bson.M{"$gte": min_level, "$lt": 5}},
		options)
	if err != nil{
		return nil, err
	}

	var entries []LogEntry
	for cursor.Next(ctx) {
		var log_entry LogEntry
		cursor.Decode(&log_entry)
		entries = append(entries, log_entry)
	}
	return entries, nil
}

func GetDetectorStatus(detector string, timeout int64) (DetectorStatus, error){
	// Gets the most recent detector status for detector. Optionally, timeout
	// ensures that the status is not stale past $timeout seconds or it will
	// set the error instead.
	collection := client.Database("daq").Collection("aggregate_status")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	options := options.Find()
	options.SetSort(bson.D{{"_id", -1}})
	options.SetLimit(1)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector}, options)

	var aggregate_status DetectorStatus
	if err != nil {
		return aggregate_status, err
	}

	for cursor.Next(ctx) {

		cursor.Decode(&aggregate_status)

		// If timeout was given a value then check
		if timeout > 0 && time.Now().Unix()-aggregate_status.Time.Unix() > timeout{
			return aggregate_status, errors.New("Status doc found, but it's " +
				strconv.FormatInt(time.Now().Unix()-aggregate_status.Time.Unix(),10) +
				" seconds old");
		}
		return aggregate_status, nil
	}
	return aggregate_status, errors.New("No status doc found at all for detector " + detector);
}
