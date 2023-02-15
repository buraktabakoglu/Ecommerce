package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	generate "github.com/buraktabakoglu/ecommerce/tokens"

	"github.com/buraktabakoglu/ecommerce/database"
	"github.com/buraktabakoglu/ecommerce/models"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

var UserCollection *mongo.Collection = database.UserData(database.Client, "Users")
var prodCollection *mongo.Collection = database.ProductData(database.Client, "Products")
var Validate = validator.New()

func HashPassword (password string) string {
	bytes , err := bcrypt.GenerateFromPassword([]byte(password),14)
	if err != nil {
		log.Panic(err)
		
	}
	return string(bytes)

}

func VerifyPassword (userPassword string, givenPassword string) (bool,string) {
	err := bcrypt.CompareHashAndPassword([]byte(givenPassword), []byte(userPassword))
	valid:=true
	msg := ""

	if err != nil{
		msg = "login or password is incorrect"
		valid = false
	}

	return valid , msg

}

func Signup () gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		if err := c.BindJSON(&user); err!=nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
			return
		}
		validationErr := Validate.Struct(user)
		if validationErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":validationErr})
			return
		}
		count, err := UserCollection.CountDocuments(ctx, bson.M{"email": user.Email})
		if err != nil {
			log.Panic(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error":err})	
			return
		}
		if count > 0 {
			c.JSON(http.StatusBadRequest,gin.H{"error":"user already exists"})
		}
		count , err = UserCollection.CountDocuments(ctx, bson.M{"phone":user.Phone})
		defer cancel()
		if  err != nil {
			log.Panic(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error":err})
			return
		}

		if count > 0 {
			c.JSON(http.StatusBadRequest,gin.H{"error":"this phone number is already in use"})
			return
		}
		password := HashPassword(*user.Password)
		user.Password = &password

		user.Created_At, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		user.Updated_At, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		user.ID = primitive.NewObjectID()
		user.User_ID = user.ID.Hex()
		token, refreshtoken, _ := generate.TokenGenerator(*user.Email, *user.First_Name,*user.Last_Name,user.User_ID)
		
		user.Token = &token
		user.Refresh_Token = &refreshtoken
		user.UserCart = make([]models.ProductUser, 0)
		user.Address_Details = make([]models.Address, 0)
		user.Order_Status = make([]models.Order, 0)
		_, inserterr := UserCollection.InsertOne(ctx,user)
		if inserterr != nil {
			c.JSON(http.StatusInternalServerError,gin.H{"error":"the user did not get created"})
			return
		}
		defer cancel()
		c.JSON(http.StatusCreated,"succesfully signed in !")

	}

}

func Login () gin.HandlerFunc {
	return func (c *gin.Context){
		var ctx , cancel = context.WithTimeout(context.Background(),100*time.Second)
		defer cancel()
		var founduser models.User

		var user models.User

		if err := c.BindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err})
			return
		}
		err := UserCollection.FindOne(ctx, bson.M{"email":user.Email}).Decode(&founduser)
		defer cancel()

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error":err})
			return
			
		}

		PasswordIsValid , msg := VerifyPassword(*user.Password,*founduser.Password)
		defer cancel()

		if !PasswordIsValid {
			c.JSON(http.StatusInternalServerError, gin.H{"error":msg})
			fmt.Println(msg)
			return
		}

		token, refreshToken, _ := generate.TokenGenerator(*founduser.Email, *founduser.First_Name, *founduser.Last_Name,founduser.User_ID)
		defer cancel()
		generate.UpdateAllTokens(token,refreshToken,founduser.User_ID)

		c.JSON(http.StatusFound,founduser)
	}

}

func ProductViewerAdmin () gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(),100*time.Second)
		var products models.Product
		defer cancel()
		if  err := c.BindJSON(&products); err != nil {
			c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
			return
			
		}
		products.Product_ID = primitive.NewObjectID()
		_, anyerr := prodCollection.InsertOne(ctx,products)
		if anyerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error":"not inserted"})
			return
		}
		defer cancel()

		c.JSON(http.StatusOK, "SUCCESSFULLY ADDED")


	}

}

func SearchProduct() gin.HandlerFunc {
	return func (c * gin.Context){
		var productList []models.Product
		var ctx , cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		cursor , err := prodCollection.Find(ctx, bson.D{{}})
		if err != nil {
			c.IndentedJSON(http.StatusInternalServerError,"something went wrong please try after some time")
			return
			
		}
		cursor.All(ctx, &productList)
		if err !=nil {
			log.Println(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
			
		}

		defer cursor.Close(ctx)
		if err := cursor.Err(); err != nil {
			log.Println(err)
			c.IndentedJSON(400,"invalid")
			return
			
		}

		defer cancel()
		c.IndentedJSON(200,productList)

	}

}

func SearchProductByQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var searchProducts []models.Product
		queryParam := c.Query("name")


		// you want to check if it's empty

		if queryParam == "" {
			log.Println("query is empty")
			c.Header("Content-Type","application/json")
			c.JSON(http.StatusNotFound, gin.H{"error":"Invalid search index"})
			c.Abort()
			return
			
		}

		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)

		defer cancel()

		searchquerydb, err := prodCollection.Find(ctx, bson.M{"product_name":bson.M{"$regex":queryParam}})
		if err != nil {
			c.IndentedJSON(404,"something went wrong thile fetching the data")
			return
		}

		searchquerydb.All(ctx, &searchProducts)
		if err !=nil {
			log.Println(err)
			c.IndentedJSON(404,"invalid")
			return
			
		}
		defer searchquerydb.Close(ctx)

		if err := searchquerydb.Err(); err != nil {
			log.Println(err)
			c.IndentedJSON(400,"invalid request")
			return
		}

		defer cancel()
		c.IndentedJSON(200,searchProducts)

	}

	
}

