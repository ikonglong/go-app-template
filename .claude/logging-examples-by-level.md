## All Logging Level Examples

The following pseudocode uses an order processing scenario to demonstrate what each log level should capture.

```
function process_order(order_details):
    // order_details = {
    //   order_id, customer_id,
    //   items: [{ product_id, product_name, quantity, unit_price, total_price }]
    // }

    // ==================== TRACE ====================
    // Finest-grained: capture the flow through the application.
    // Log entry/exit of methods, intermediate variable states,
    // iteration steps — things only needed when tracing exact execution path.

    TRACE("Entering process_order, order_id={}, customer_id={}, item_count={}",
          order_details.order_id, order_details.customer_id, len(order_details.items))

    for item in order_details.items:
        TRACE("Processing item: product_id={}, product_name={}, quantity={}, unit_price={}, total_price={}",
              item.product_id, item.product_name, item.quantity, item.unit_price, item.total_price)

        stock = get_product_stock(item.product_id)
        TRACE("Stock lookup result: product_id={}, available_stock={}, requested_quantity={}",
              item.product_id, stock.available, item.quantity)

        if stock.available < item.quantity:
            TRACE("Insufficient stock detected, about to throw error for product_id={}", item.product_id)
            throw InsufficientStockError(item.product_id, stock.available, item.quantity)

    subtotal = sum(item.total_price for item in order_details.items)
    TRACE("Subtotal calculated: {}", subtotal)

    discount = calculate_discount(order_details)
    TRACE("Discount calculation returned: discount_amount={}, rule_applied={}",
          discount.amount, discount.rule_name)

    final_total = subtotal - discount.amount
    TRACE("Final total computed: (subtotal={}) - (discount={}) = (final_total={})",
          subtotal, discount.amount, final_total)

    order = create_order(order_details, final_total)
    TRACE("Exiting process_order, created order internal_id={}", order.internal_id)

    return order


    // ==================== DEBUG ====================
    // Fine-grained: useful for debugging. Log meaningful decision points,
    // computed values, and data that helps reconstruct what happened
    // without logging every single step.

    DEBUG("Processing order: order_id={}, customer_id={}, items={}",
          order_details.order_id, order_details.customer_id,
          [{ product_id: i.product_id, qty: i.quantity } for i in order_details.items])

    for item in order_details.items:
        stock = get_product_stock(item.product_id)
        DEBUG("Stock check: product_id={}, available={}, requested={}",
              item.product_id, stock.available, item.quantity)

        if stock.available < item.quantity:
            throw InsufficientStockError(item.product_id, stock.available, item.quantity)

    subtotal = sum(item.total_price for item in order_details.items)
    discount = calculate_discount(order_details)
    final_total = subtotal - discount.amount

    DEBUG("Order pricing: subtotal={}, discount_amount={} (rule={}), final_total={}",
          subtotal, discount.amount, discount.rule_name, final_total)

    order = create_order(order_details, final_total)
    DEBUG("Order created: order_id={}, db_id={}", order_details.order_id, order.internal_id)

    return order


    // ==================== INFO ====================
    // Coarse-grained progress: highlight key business milestones.
    // An operator reading INFO logs should understand WHAT happened
    // at a business level without being overwhelmed.

    INFO("Order processing started: order_id={}, customer_id={}, item_count={}",
         order_details.order_id, order_details.customer_id, len(order_details.items))

    for item in order_details.items:
        stock = get_product_stock(item.product_id)
        if stock.available < item.quantity:
            throw InsufficientStockError(item.product_id, stock.available, item.quantity)

    subtotal = sum(item.total_price for item in order_details.items)
    discount = calculate_discount(order_details)
    final_total = subtotal - discount.amount

    order = create_order(order_details, final_total)

    INFO("Order created successfully: order_id={}, final_total={}, discount={}",
         order_details.order_id, final_total, discount.amount)

    return order


    // ==================== WARN ====================
    // Potentially harmful situations that might lead to errors.
    // The operation still succeeds, but something smells off
    // and deserves attention before it becomes a real problem.

    for item in order_details.items:
        stock = get_product_stock(item.product_id)

        if stock.available < item.quantity:
            throw InsufficientStockError(item.product_id, stock.available, item.quantity)

        if stock.available - item.quantity <= stock.low_threshold:
            WARN("Low stock after order: product_id={}, remaining={}, threshold={}, order_id={}",
                 item.product_id, stock.available - item.quantity, stock.low_threshold,
                 order_details.order_id)

    subtotal = sum(item.total_price for item in order_details.items)
    discount = calculate_discount(order_details)
    final_total = subtotal - discount.amount

    if discount.amount > subtotal * 0.5:
        WARN("Unusually large discount applied: order_id={}, discount={}, subtotal={}, rule={}",
             order_details.order_id, discount.amount, subtotal, discount.rule_name)

    if len(order_details.items) > 50:
        WARN("Order has unusually high item count: order_id={}, item_count={} — may impact processing time",
             order_details.order_id, len(order_details.items))

    order = create_order(order_details, final_total)
    return order


    // ==================== ERROR ====================
    // An error event that might still allow the application to continue.
    // The current operation failed, but the system is still alive.
    // Log enough context to diagnose WITHOUT requiring a reproducer.

    try:
        for item in order_details.items:
            stock = get_product_stock(item.product_id)
            if stock.available < item.quantity:
                ERROR("Insufficient stock: order_id={}, product_id={}, requested={}, available={}, customer_id={}",
                      order_details.order_id, item.product_id, item.quantity, stock.available,
                      order_details.customer_id)
                throw InsufficientStockError(item.product_id, stock.available, item.quantity)

        subtotal = sum(item.total_price for item in order_details.items)
        discount = calculate_discount(order_details)
        final_total = subtotal - discount.amount
        order = create_order(order_details, final_total)
        return order

    catch DatabaseException as e:
        ERROR("Failed to create order: order_id={}, customer_id={}, error={}, items={}",
              order_details.order_id, order_details.customer_id, e.message,
              [{ product_id: i.product_id, qty: i.quantity } for i in order_details.items])
        throw OrderCreationFailedError(order_details.order_id, e)

    catch DiscountServiceException as e:
        ERROR("Discount calculation failed, proceeding without discount: order_id={}, error={}",
              order_details.order_id, e.message)
        final_total = subtotal  // fallback: no discount
        order = create_order(order_details, final_total)
        return order


    // ==================== FATAL ====================
    // So severe the application should abort. The system is in an
    // unrecoverable state — log everything needed for post-mortem,
    // because there may be no second chance.

    try:
        for item in order_details.items:
            stock = get_product_stock(item.product_id)
            if stock.available < item.quantity:
                throw InsufficientStockError(...)

        subtotal = sum(item.total_price for item in order_details.items)
        discount = calculate_discount(order_details)
        final_total = subtotal - discount.amount
        order = create_order(order_details, final_total)
        return order

    catch ConnectionPoolExhausted as e:
        FATAL("Database connection pool exhausted, cannot process any orders. " +
              "pool_size={}, active={}, waiting={}, last_order_attempted={}",
              e.pool_size, e.active_count, e.waiting_count, order_details.order_id)
        // FATAL events should trigger immediate alerts.
        // FATAL only logs — it does NOT auto-shutdown the application.
        // Whether to shutdown, and how, is the developer's decision.
        throw e

    catch DataCorruptionException as e:
        FATAL("Critical data integrity violation in order pipeline: order_id={}, " +
              "table={}, constraint={}, customer_id={}. Halting order processing to prevent further corruption.",
              order_details.order_id, e.table, e.constraint, order_details.customer_id)
        // FATAL events should trigger immediate alerts.
        // FATAL only logs — it does NOT auto-shutdown the application.
        // Whether to shutdown, and how, is the developer's decision.
        throw e
```
