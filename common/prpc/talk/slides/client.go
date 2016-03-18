var client GreeterClient = ...
res, err := client.SayHello(ctx, &HelloRequest{"Nodir"})
if err != nil {
  // switch on error code...
  return err
}
fmt.Println(res.Message)