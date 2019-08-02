package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
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
