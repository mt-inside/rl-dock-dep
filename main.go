package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	d := NewDeployments()

	NewReconsiler(d)

	r := gin.Default()

	r.GET("/deployments", d.ListDeployments)
	r.GET("/deployment/:id", d.GetDeployment)
	r.POST("/deployments", d.MakeDeployment)
	r.PATCH("/deployment/:id", d.UpdateDeployment)
	r.DELETE("/deployment/:id", d.DeleteDeployment)

	r.Run()

}
