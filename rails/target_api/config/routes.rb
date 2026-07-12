Rails.application.routes.draw do
  post "/orders/replicate", to: "replications#create"
  get "/orders/:id", to: "orders#show"
  get "/healthz", to: "healthz#show"
end
