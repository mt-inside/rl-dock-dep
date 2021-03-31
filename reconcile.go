package main

import (
	"context"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type Reconsiler struct {
	docker *client.Client
	deps   *Deployments
	q      <-chan struct{}
	evts   <-chan events.Message
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewReconsiler(deps *Deployments) *Reconsiler {
	docker, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	filters := filters.NewArgs()
	filters.Add("type", events.ContainerEventType)
	filters.Add("action", "destroy") // doesn't work
	evts, errs := docker.Events(context.TODO(), types.EventsOptions{Filters: filters})
	go func() {
		for e := range errs {
			log.Printf("Docker error: %v", e)
		}
	}()

	q := make(chan struct{})
	r := &Reconsiler{docker, deps, q, evts}
	go r.loop()
	deps.Subscribe(q)

	return r
}

func (r *Reconsiler) loop() {
	for {
		select {
		case dockerEvent := <-r.evts:
			if dockerEvent.Type != "container" || dockerEvent.Action != "destroy" {
				continue
			}
			log.Printf("Docker event: %v", dockerEvent)
		case <-r.q:
			log.Printf("Model event")
		}

		log.Print("Reconsiling")

		/* Desired state */

		desired := r.deps.GetAll()

		/* Actual state */

		cs, err := r.docker.ContainerList(context.TODO(), types.ContainerListOptions{All: true})
		if err != nil {
			log.Printf("Error reading world: %v", err)
			continue
		}
		byOwner := map[string][]types.Container{}
		for _, c := range cs {
			owner, ok := c.Labels["owner"]
			if !ok {
				log.Printf("Container %s %v %s not one of ours; ignoring\n", c.ID[:10], c.Names, c.Image)
				continue
			}
			byOwner[owner] = append(byOwner[owner], c)
		}
		for o, cs := range byOwner {
			for _, c := range cs {
				log.Printf("Owner: %s, Names: %s\n", o, c.Names)
			}
		}

		/* Bring into line */

		for _, d := range desired {
			world := byOwner[d.Id.String()]
			nDesired := d.Replicas
			nWorld := len(world)

			more := nDesired - nWorld
			if more > 0 {
				for i := 0; i < more; i++ {
					r.makeContainer(d)
					// Errors not important; we'll just try again to close the gap next time round the loop
				}
			}
			if more < 0 {
				for i := 0; i < -more; i++ {
					r.deleteContainer(world[i])
					// Errors not important; we'll just try again to close the gap next time round the loop
				}
			}

			delete(byOwner, d.Id.String())
		}

		// Anything now left in byOwner has an owner label, so it's managed by us, but is no-longer desired; so delete.
		// This has the happy side-effect of cleaning up orphaned containers from previous invocations.
		for _, owned := range byOwner {
			for _, o := range owned {
				r.deleteContainer(o)
			}
		}

		// TODO: Schedule another reconciliation, since I cba to wait for ack / check results, think of this as a retry. If that next loop has to do something (ie try again), it'll also scheudle another spin, hence another retry
	}
}

func (r *Reconsiler) makeContainer(dep Deployment) {
	name := dep.Name + "-" + strconv.Itoa(100000+rand.Intn(900000))
	log.Printf("Making %s", name)

	c, err := r.docker.ContainerCreate(
		context.TODO(),
		&container.Config{
			Image:  dep.Image,
			Labels: map[string]string{"owner": dep.Id.String()},
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		&specs.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		name,
	)
	if err != nil {
		log.Printf("Error making container: %v", err)
	}

	r.docker.ContainerStart(
		context.TODO(),
		c.ID,
		types.ContainerStartOptions{},
	)
}

func (r *Reconsiler) deleteContainer(c types.Container) {
	log.Printf("Deleting %s\n", c.Names)

	r.docker.ContainerRemove(
		context.TODO(),
		c.ID,
		types.ContainerRemoveOptions{
			Force: true,
		},
	)
}
