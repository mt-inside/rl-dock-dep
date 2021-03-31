package main

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Deployments struct {
	lock         sync.Mutex
	deps         map[uuid.UUID]Deployment
	changedQueue chan<- struct{}
}

type createDepCommand struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas int    `json:"replicas"`
}

type Deployment struct {
	Id       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Image    string    `json:"image"`
	Replicas int       `json:"replicas"`
}

func (d Deployment) String() string {
	return fmt.Sprintf("%s (%s) * %d", d.Name, d.Image, d.Replicas)
}

func NewDeployments() *Deployments {
	return &Deployments{deps: make(map[uuid.UUID]Deployment)}
}

func (d *Deployments) GetAll() []Deployment {
	d.lock.Lock()
	defer d.lock.Unlock()

	ds := make([]Deployment, len(d.deps))
	i := 0
	for _, d := range d.deps {
		ds[i] = d
		i = i + 1
	}

	return ds
}

func (d *Deployments) ListDeployments(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "deployments": d.GetAll()})
}

func (d *Deployments) findDeployment(c *gin.Context) (*Deployment, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	dep, found := d.deps[id]
	if !found {
		return nil, errors.New("id not found")
	}

	return &dep, nil
}

func (d *Deployments) GetDeployment(c *gin.Context) {
	dep, err := d.findDeployment(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": err.Error()})
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "deployment": dep})
}

func (d *Deployments) saveDeployment(c *gin.Context, id uuid.UUID) (*Deployment, error) {
	var in createDepCommand
	if err := c.ShouldBindJSON(&in); err != nil {
		return nil, err
	}

	dep := Deployment{Id: id, Name: in.Name, Image: in.Image, Replicas: in.Replicas}
	// TODO: enforce that only replicas can change (needs refactor)

	d.lock.Lock()
	defer d.lock.Unlock()
	d.deps[dep.Id] = dep
	d.emitChanged()

	return &dep, nil
}

func (d *Deployments) MakeDeployment(c *gin.Context) {
	dep, err := d.saveDeployment(c, uuid.New())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "deployment": dep})
}

func (d *Deployments) UpdateDeployment(c *gin.Context) {
	dep, err := d.findDeployment(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": err.Error()})
	}

	dep, err = d.saveDeployment(c, dep.Id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": err.Error()})
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "deployment": dep})
}

func (d *Deployments) DeleteDeployment(c *gin.Context) {
	dep, err := d.findDeployment(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": err.Error()})
	}

	d.lock.Lock()
	defer d.lock.Unlock()
	delete(d.deps, dep.Id)
	d.emitChanged()

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (d *Deployments) Subscribe(q chan<- struct{}) {
	if d.changedQueue != nil {
		panic("We're not that smart")
	}
	d.changedQueue = q
}

func (d *Deployments) emitChanged() {
	if d.changedQueue != nil {
		d.changedQueue <- struct{}{}
	}
}
