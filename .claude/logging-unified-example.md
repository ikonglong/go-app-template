## Unified Example (Production Style)

In real code, all log levels coexist in a single function. Since log frameworks are
hierarchical (e.g., setting level to DEBUG shows DEBUG + INFO + WARN + ERROR + FATAL),
lower levels should **add detail**, not **repeat** what higher levels already say. This is the **key principle**.

```
function process_order(order_details):

    // --- Entry: TRACE adds full input detail; INFO only marks the milestone ---
    TRACE("Entering process_order, order_id={}, customer_id={}, item_count={}",
          order_details.order_id, order_details.customer_id, len(order_details.items))
    // No need for INFO to repeat order_id + customer_id here —
    // INFO logs the same fields at the end on success. One INFO per business
    // milestone is enough; two would be noise at this level.

    try:
        for item in order_details.items:
            // TRACE: per-item full detail (all fields)
            TRACE("Processing item: product_id={}, product_name={}, quantity={}, unit_price={}, total_price={}",
                  item.product_id, item.product_name, item.quantity, item.unit_price, item.total_price)

            stock = get_product_stock(item.product_id)

            // TRACE: raw lookup result — DEBUG summarizes the decision point
            TRACE("Stock lookup result: product_id={}, available_stock={}, requested_quantity={}",
                  item.product_id, stock.available, item.quantity)
            // DEBUG: only the decision-relevant fields. No need to repeat product_name,
            // unit_price, etc. — TRACE already has them if you need to dig deeper.
            DEBUG("Stock check: product_id={}, available={}, requested={}",
                  item.product_id, stock.available, item.quantity)

            if stock.available < item.quantity:
                // ERROR: full diagnostic context for this failure.
                // Includes customer_id (not in DEBUG/TRACE) because error diagnosis
                // often needs to know WHO was affected.
                ERROR("Insufficient stock: order_id={}, product_id={}, requested={}, available={}, customer_id={}",
                      order_details.order_id, item.product_id, item.quantity, stock.available,
                      order_details.customer_id)
                // No DEBUG/TRACE here — the ERROR already contains all needed context,
                // and the preceding DEBUG "Stock check" line provides the trail.
                throw InsufficientStockError(item.product_id, stock.available, item.quantity)

            // WARN: operation succeeded, but stock is dangerously low
            if stock.available - item.quantity <= stock.low_threshold:
                WARN("Low stock after order: product_id={}, remaining={}, threshold={}, order_id={}",
                     item.product_id, stock.available - item.quantity, stock.low_threshold,
                     order_details.order_id)

        subtotal = sum(item.total_price for item in order_details.items)
        TRACE("Subtotal calculated: {}", subtotal)

        discount = calculate_discount(order_details)
        // TRACE: intermediate step; DEBUG: the meaningful summary
        TRACE("Discount calculation returned: discount_amount={}, rule_applied={}",
              discount.amount, discount.rule_name)

        final_total = subtotal - discount.amount
        // DEBUG: pricing summary — subsumes the TRACE subtotal/discount lines above.
        // A developer debugging at DEBUG level sees one line with all pricing info
        // instead of three separate TRACE lines.
        DEBUG("Order pricing: subtotal={}, discount_amount={} (rule={}), final_total={}",
              subtotal, discount.amount, discount.rule_name, final_total)

        // WARN: anomaly detection — only fires conditionally, no overlap with normal-path logs
        if discount.amount > subtotal * 0.5:
            WARN("Unusually large discount: order_id={}, discount={}, subtotal={}, rule={}",
                 order_details.order_id, discount.amount, subtotal, discount.rule_name)

        order = create_order(order_details, final_total)

        TRACE("Exiting process_order, created order internal_id={}", order.internal_id)
        // DEBUG: adds internal_id which INFO doesn't need
        DEBUG("Order created: order_id={}, db_id={}", order_details.order_id, order.internal_id)
        // INFO: the one business milestone — coarse summary for operators.
        // Does NOT repeat subtotal, discount rule, db_id — those are DEBUG/TRACE concerns.
        INFO("Order created successfully: order_id={}, final_total={}, discount={}",
             order_details.order_id, final_total, discount.amount)

        return order

    catch DatabaseException as e:
        ERROR("Failed to create order: order_id={}, customer_id={}, error={}, items={}",
              order_details.order_id, order_details.customer_id, e.message,
              [{ product_id: i.product_id, qty: i.quantity } for i in order_details.items])
        throw OrderCreationFailedError(order_details.order_id, e)

    catch DiscountServiceException as e:
        ERROR("Discount calculation failed, proceeding without discount: order_id={}, error={}",
              order_details.order_id, e.message)
        final_total = subtotal
        order = create_order(order_details, final_total)
        INFO("Order created successfully: order_id={}, final_total={}, discount=0",
             order_details.order_id, final_total)
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
