package main

import (
  "net/http"

  "github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
  // Initialize a new httprouter router instance.
  router := httprouter.New()

  // Convert the notFoundResponse() helper to a http.Handler using the
  // http.HandlerFunc() adapter, and then set it as the custom error handler
  // for 404 Not Found responses.
  router.NotFound = http.HandlerFunc(app.notFoundResponse)

  // Likewise, convert the methodNotAllowedResponse() helper to a http.Handler
  // and set it as the custom error handler for 405 Method Not Allowed
  // responses.
  router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

  // Register the relevant methods, URL patterns and handler functions for our
  // endpoints using the HandlerFunc() method. Note that http.MethodGet and
  // http.MethodPost are constants which equate to the strings "GET" and "POST"
  // respectively.
  router.HandlerFunc(http.MethodGet, "/v1/movies", app.listMoviesHandler)
  router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)
  router.HandlerFunc(http.MethodPost, "/v1/movies", app.createMovieHandler)
  router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.showMovieHandler)
  // Add the route for the PATCH /v1/movies/:id endpoint.
  router.HandlerFunc(http.MethodPatch, "/v1/movies/:id", app.updateMovieHandler)
  // Add the route for the DELETE /v1/movies/:id endpoint
  router.HandlerFunc(http.MethodDelete, "/v1/movies/:id", app.deleteMovieHandler)

  // Add the route for the POST /v1/users endpoint.
  router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
  // Add the route for the PUT /v1/users/activated endpoint.
  router.HandlerFunc(http.MethodPut, "/v1/users/activated",
    app.activateUserHandler)

  // Add the route for the POST /v1/tokens/authentication endpoint.
  router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication",
    app.createAuthenticationTokenHandler)

  // Return the httprouter instance.
  return app.recoverPanic(app.rateLimit(app.authenticate(router)))
}
