Rails.application.routes.draw do
  post "/orders", to: "orders#create"
  get "/healthz", to: "healthz#show"
end
