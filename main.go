package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var MONGODB_URI string = os.Getenv("MONGODB_URI")
var mongoClient *mongo.Client

const DB string = "fdroidl10n"

var apps_collection *mongo.Collection
var values_collection *mongo.Collection

// The init function will run before our main function to establish a connection to MongoDB. If it cannot connect it will fail and the program will exit.
func init() {
	if err := connect_to_mongodb(); err != nil {
		log.Fatal("Could not connect to MongoDB")
	}
	log.Println("Connected to MongoDB!")
}

func connect_to_mongodb() error {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(MONGODB_URI).SetServerAPIOptions(serverAPI)

	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		panic(err)
	}
	err = client.Ping(context.TODO(), nil)
	mongoClient = client
	apps_collection = client.Database(DB).Collection("apps")
	values_collection = client.Database(DB).Collection("values")
	return err
}

func main() {
	fmt.Println("Starting server...")
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	r.GET("/robots.txt", func(ctx *gin.Context) {
		ctx.String(200, "User-agent: *\nWelcome!\n")
	})
	r.GET("/favicon.ico", func(ctx *gin.Context) {
		// 301 ->
		// "static/fdroid-icon.png"
		ctx.Redirect(301, "/static/fdroid-icon.png")
	})
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	r.GET("/static/*filepath", func(c *gin.Context) {
		c.File("static/" + c.Param("filepath"))
	})

	r.GET("/api/stats/values", func(c *gin.Context) {
		pipeline := []bson.M{
			{
				"$group": bson.M{
					"_id":            "$valuesName",
					"count":          bson.M{"$sum": 1},
					"stringCountSum": bson.M{"$sum": "$stringCount"},
				},
			},
		}

		cursor, err := values_collection.Aggregate(context.Background(), pipeline)
		if err != nil {
			log.Fatal(err)
		}

		var results []bson.M
		if err := cursor.All(context.Background(), &results); err != nil {
			log.Fatal(err)
		}

		var resultMap = make(map[string]bson.M)
		for _, result := range results {
			result_id := result["_id"].(string)
			delete(result, "_id")
			resultMap[result_id] = result
		}

		c.JSON(200, resultMap)
	})

	r.GET("/api/stats/db", func(c *gin.Context) {
		// datasize and storageSize
		cmd := bson.D{{Key: "dbStats", Value: 1}}
		result := bson.M{}
		err := mongoClient.Database(DB).RunCommand(context.Background(), cmd).Decode(&result)
		if err != nil {
			log.Fatal(err)
		}
		apps_docs, err := apps_collection.CountDocuments(context.Background(), bson.M{})
		if err != nil {
			log.Fatal(err)
		}
		values_docs, err := values_collection.CountDocuments(context.Background(), bson.M{})
		if err != nil {
			log.Fatal(err)
		}

		// set cache
		c.Header("Cache-Control", "public, max-age=86400")

		c.JSON(200, gin.H{
			"apps_docs":      apps_docs,
			"values_docs":    values_docs,
			"db_datasize":    result["dataSize"],
			"db_storageSize": result["storageSize"],
		})
	})

	// Redirect to /app/:packageName/versionCode/{maxVersionCode}/*any
	r.GET("/app/:packageName/versionCode/latest/*any", func(c *gin.Context) {
		packageName := c.Param("packageName")
		any := c.Param("any")

		// find latest versionCode
		var result = bson.M{}
		err := apps_collection.FindOne(context.Background(), bson.M{"packageName": packageName}, options.FindOne().SetSort(bson.M{"versionCode": -1})).Decode(&result)
		if err != nil {
			log.Println(err)
			c.HTML(404, "404.html", nil)
			return
		}
		versionCode := result["versionCode"].(int64)

		c.Redirect(302, fmt.Sprintf("/app/%s/versionCode/%d/%s", packageName, versionCode, any))
	})

	// {
	// 	_id: ObjectId('6581c9861e1bd12a293cb3c6'),
	// 	packageName: 'name.juodumas.ext_kbd_lithuanian',
	// 	sourceCode: 'https://github.com/juodumas/android-lithuanian-layouts',
	// 	versionCode: Long('1'),
	// 	versionName: '1.0',
	// 	fileName: '/name.juodumas.ext_kbd_lithuanian_1.apk',
	// 	status: 'DONE'
	//   }
	r.GET("/app/:packageName/versionCode/:versionCode", func(c *gin.Context) {

		packageName := c.Param("packageName")
		var versionCode int64

		if c.Param("versionCode") == "latest" {
			// find latest versionCode
			var result = bson.M{}
			err := apps_collection.FindOne(context.Background(), bson.M{"packageName": packageName}, options.FindOne().SetSort(bson.M{"versionCode": -1})).Decode(&result)
			if err != nil {
				log.Println(err)
				c.HTML(404, "404.html", nil)
				return
			}
			versionCode = result["versionCode"].(int64)
			c.Redirect(302, fmt.Sprintf("./%d", versionCode))
			return
		} else {
			var err error
			// int64
			versionCode, err = strconv.ParseInt(c.Param("versionCode"), 10, 64)
			if err != nil {
				c.HTML(404, "404.html", nil)
				return
			}
		}

		var result = bson.M{}

		// find all {packageName: packageName, versionCode: versionCode}
		err := apps_collection.FindOne(context.Background(), bson.M{"packageName": packageName, "versionCode": versionCode}).Decode(&result)
		if err != nil {
			log.Println(err)
			c.HTML(404, "404.html", nil)
			return
		}
		c.HTML(200, "app_versionCode.html", gin.H{
			"packageName": packageName,
			"versionCode": versionCode,
			"sourceCode":  result["sourceCode"],
			"versionName": result["versionName"],
			"status":      result["status"],
		})
	})

	/*
		Query Parameters:
		packageName: "com.abcd.efgh"
		versionCode: 1 int64
		valuesName: "values*"
	*/
	// http://localhost:8080/api/app_values?packageName=me.zhanghai.android.untracker&versionCode=1&valuesName=values
	r.GET("/api/app_values", func(c *gin.Context) {
		packageName := c.Query("packageName")
		log.Println(packageName)
		versionCode, err := strconv.ParseInt(c.Query("versionCode"), 10, 64)
		log.Println(versionCode)
		if err != nil {
			c.JSON(404, gin.H{
				"message": "versionCode is not int64",
			})
			return
		}
		valuesName := c.Query("valuesName")
		var valuesResult bson.M
		err = values_collection.FindOne(context.Background(), bson.M{"packageName": packageName, "versionCode": versionCode, "valuesName": valuesName}).Decode(&valuesResult)
		if err != nil {
			c.JSON(404, gin.H{
				"message": "values not found",
			})
			return
		}
		// set cache
		c.Header("Cache-Control", "public, max-age=86400")
		c.JSON(200, gin.H{
			"data": valuesResult,
		})
	})

	r.GET("/app/:packageName/versionCode/:versionCode/values", func(c *gin.Context) {
		packageName := c.Param("packageName")
		versionCode, err := strconv.ParseInt(c.Param("versionCode"), 10, 64)
		if err != nil {
			c.HTML(404, "404.html", nil)
			return
		}
		// 查 Mongo，获取 app 数据
		var appResult bson.M
		err = apps_collection.FindOne(context.Background(), bson.M{"packageName": packageName, "versionCode": versionCode}).Decode(&appResult)
		if err != nil {
			log.Println(err)
			c.HTML(404, "404.html", nil)
			return
		}

		// 查 Mongo，获取 values 数据
		log.Println(packageName, versionCode)
		var valuesResults []bson.M
		cursor, err := values_collection.Find(context.Background(), bson.M{"packageName": packageName, "versionCode": versionCode})
		if err != nil {
			log.Println(err)
			c.HTML(404, "404.html", nil)
			return
		}
		if err = cursor.All(context.Background(), &valuesResults); err != nil {
			log.Fatal(err)
		}

		// 计算最大的 stringCount
		// int32
		var maxStringCount int32 = 0
		for _, result := range valuesResults {
			if result["stringCount"].(int32) > maxStringCount {
				maxStringCount = result["stringCount"].(int32)
			}
		}
		log.Println(maxStringCount)

		// for valuesResult, add percentage [max: 100]
		for i, result := range valuesResults {
			valuesResults[i]["percentage"] = float32(result["stringCount"].(int32)) / float32(maxStringCount) * 100
		}

		c.HTML(200, "app_versionCode_values.html", gin.H{
			"packageName":    packageName,
			"versionCode":    versionCode,
			"sourceCode":     appResult["sourceCode"],
			"values":         valuesResults,
			"len_values":     len(valuesResults),
			"maxStringCount": maxStringCount,
		})
	})

	// default 404
	r.NoRoute(func(c *gin.Context) {
		c.HTML(404, "404.html", nil)
	})

	r.Run()
}
